package cacheStore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/redisutil"
	"myai-url-shortener/internal/shortener/store"

	"github.com/redis/go-redis/v9"
)

type Options struct {
	Prefix             string
	TTL                time.Duration
	LockTTL            time.Duration
	LockWait           time.Duration
	VisitSyncInterval  time.Duration
	VisitSyncQueueSize int
}

type CachedStore struct {
	next        store.Store
	redis       *redisutil.Client
	prefix      string
	ttl         time.Duration
	lockTTL     time.Duration
	lockWait    time.Duration
	visitSyncer *VisitSyncer
}

var incrementScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])

if redis.call("EXISTS", key) == 0 then
  return {"MISS", "not_found"}
end

local is_deleted = redis.call("HGET", key, "is_deleted")
if is_deleted == "1" or is_deleted == "true" then
  return {"ERR", "deleted"}
end

local expires_at = tonumber(redis.call("HGET", key, "expires_at") or "0")
if expires_at > 0 and expires_at <= now then
  return {"ERR", "expired"}
end

local visits = tonumber(redis.call("HGET", key, "visits") or "0")
local max_visits = tonumber(redis.call("HGET", key, "max_visits") or "0")
if max_visits > 0 and visits >= max_visits then
  return {"ERR", "visits_exhausted"}
end

visits = redis.call("HINCRBY", key, "visits", 1)
redis.call("HSET", key, "updated_at", now)
return {"OK", tostring(visits)}
`)

func NewCachedStore(next store.Store, client redis.UniversalClient, options Options) *CachedStore {
	prefix := strings.TrimSpace(options.Prefix)
	if prefix == "" {
		prefix = "myai:url-shortener"
	}
	ttl := options.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	lockTTL := options.LockTTL
	if lockTTL <= 0 {
		lockTTL = 3 * time.Second
	}
	lockWait := options.LockWait
	if lockWait <= 0 {
		lockWait = 120 * time.Millisecond
	}

	var redisClient *redisutil.Client
	if client != nil {
		redisClient = redisutil.New(client)
	}
	var visitSyncer *VisitSyncer
	if next != nil {
		visitSyncer = NewVisitSyncer(next, VisitSyncerOptions{
			FlushInterval: options.VisitSyncInterval,
			QueueSize:     options.VisitSyncQueueSize,
		})
	}

	return &CachedStore{
		next:        next,
		redis:       redisClient,
		prefix:      prefix,
		ttl:         ttl,
		lockTTL:     lockTTL,
		lockWait:    lockWait,
		visitSyncer: visitSyncer,
	}
}

func (s *CachedStore) Create(ctx context.Context, link shortener.Link) error {
	if err := s.next.Create(ctx, link); err != nil {
		return err
	}
	s.setLink(ctx, link)
	return nil
}

func (s *CachedStore) Get(ctx context.Context, code string) (shortener.Link, error) {
	if link, ok := s.getLink(ctx, code); ok {
		return link, nil
	}

	if link, ok, err := s.loadLinkWithLock(ctx, code); err != nil {
		return shortener.Link{}, err
	} else if ok {
		return link, nil
	}

	link, err := s.next.Get(ctx, code)
	if err != nil {
		return shortener.Link{}, err
	}
	s.setLink(ctx, link)
	return link, nil
}

func (s *CachedStore) IncrementVisits(ctx context.Context, code string) (shortener.Link, error) {
	if s.redis == nil {
		return s.next.IncrementVisits(ctx, code)
	}

	key := s.linkKey(code)
	status, reason, err := s.runIncrementScript(ctx, key)
	if err != nil || status == "MISS" {
		link, fallbackErr := s.next.IncrementVisits(ctx, code)
		if fallbackErr != nil {
			return shortener.Link{}, fallbackErr
		}
		s.setLink(ctx, link)
		return link, nil
	}
	if status == "ERR" {
		return shortener.Link{}, redisReasonError(reason)
	}

	if s.visitSyncer == nil || !s.visitSyncer.Enqueue(code) {
		log.Printf("visit sync queue is full, falling back to sync mongo update: code=%s", code)
		if err := incrementVisitsBy(ctx, s.next, code, 1); err != nil {
			_ = s.redis.Del(ctx, key)
			return shortener.Link{}, err
		}
	}

	link, ok := s.getLink(ctx, code)
	if !ok {
		return s.next.Get(ctx, code)
	}
	return link, nil
}

func (s *CachedStore) List(ctx context.Context) ([]shortener.Link, error) {
	return s.next.List(ctx)
}

func (s *CachedStore) Delete(ctx context.Context, code string) (shortener.Link, error) {
	link, err := s.next.Delete(ctx, code)
	if err != nil {
		if s.redis != nil && errors.Is(err, store.ErrLinkNotFound) {
			_ = s.redis.Del(ctx, s.linkKey(code))
		}
		return shortener.Link{}, err
	}
	if s.redis != nil {
		_ = s.redis.Del(ctx, s.linkKey(code))
	}
	return link, nil
}

func (s *CachedStore) Close(ctx context.Context) error {
	if s.visitSyncer == nil {
		return nil
	}
	return s.visitSyncer.Close(ctx)
}

func (s *CachedStore) loadLinkWithLock(ctx context.Context, code string) (shortener.Link, bool, error) {
	if s.redis == nil {
		return shortener.Link{}, false, nil
	}

	lock, acquired, err := s.redis.AcquireLock(ctx, s.lockKey(code), s.lockTTL)
	if err != nil {
		return shortener.Link{}, false, nil
	}
	if acquired {
		defer releaseLock(lock)

		if link, ok := s.getLink(ctx, code); ok {
			return link, true, nil
		}
		link, err := s.next.Get(ctx, code)
		if err != nil {
			return shortener.Link{}, false, err
		}
		s.setLink(ctx, link)
		return link, true, nil
	}

	deadline := time.NewTimer(s.lockWait)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return shortener.Link{}, false, ctx.Err()
		case <-ticker.C:
			if link, ok := s.getLink(ctx, code); ok {
				return link, true, nil
			}
		case <-deadline.C:
			return shortener.Link{}, false, nil
		}
	}
}

func (s *CachedStore) runIncrementScript(ctx context.Context, key string) (string, string, error) {
	result, err := s.redis.RunScript(ctx, incrementScript, []string{key}, time.Now().Unix())
	if err != nil {
		return "", "", err
	}

	values, ok := result.([]interface{})
	if !ok || len(values) < 2 {
		return "", "", errors.New("unexpected redis script response")
	}

	status := fmt.Sprint(values[0])
	reason := fmt.Sprint(values[1])
	return status, reason, nil
}

func redisReasonError(reason string) error {
	switch reason {
	case "deleted", "not_found":
		return store.ErrLinkNotFound
	case "expired":
		return store.ErrLinkExpired
	case "visits_exhausted":
		return store.ErrVisitsExhausted
	default:
		return errors.New(reason)
	}
}

func (s *CachedStore) getLink(ctx context.Context, code string) (shortener.Link, bool) {
	if s.redis == nil {
		return shortener.Link{}, false
	}

	values, err := s.redis.HGetAll(ctx, s.linkKey(code))
	if err != nil || len(values) == 0 {
		return shortener.Link{}, false
	}

	link := linkFromHash(values)
	if !cacheable(link, time.Now()) {
		_ = s.redis.Del(ctx, s.linkKey(code))
		return shortener.Link{}, false
	}
	return link, true
}

func (s *CachedStore) setLink(ctx context.Context, link shortener.Link) {
	if s.redis == nil || !cacheable(link, time.Now()) {
		return
	}

	key := s.linkKey(link.Code)
	_ = s.redis.HSet(ctx, key, linkHash(link))
	_ = s.redis.Expire(ctx, key, s.linkTTL(link, time.Now()))
}

func (s *CachedStore) linkTTL(link shortener.Link, now time.Time) time.Duration {
	ttl := s.ttl
	if link.ExpiresAt == nil {
		return ttl
	}

	untilExpire := link.ExpiresAt.Sub(now)
	if untilExpire <= 0 {
		return time.Second
	}
	if untilExpire < ttl {
		return untilExpire
	}
	return ttl
}

func (s *CachedStore) linkKey(code string) string {
	return s.prefix + ":link:" + strings.TrimSpace(code)
}

func (s *CachedStore) lockKey(code string) string {
	return s.prefix + ":lock:link:" + strings.TrimSpace(code)
}

func releaseLock(lock *redisutil.Lock) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lock.Release(ctx); err != nil {
		log.Printf("release redis lock failed: %v", err)
	}
}

func cacheable(link shortener.Link, now time.Time) bool {
	if link.Code == "" || link.IsDeleted {
		return false
	}
	if link.ExpiresAt != nil && !link.ExpiresAt.After(now) {
		return false
	}
	return true
}

func linkHash(link shortener.Link) map[string]interface{} {
	return map[string]interface{}{
		"code":                link.Code,
		"kind":                link.Kind,
		"url":                 link.URL,
		"title":               link.Title,
		"scope":               link.Scope,
		"visits":              link.Visits,
		"max_visits":          link.MaxVisits,
		"created_at":          unixOrZero(link.CreatedAt),
		"updated_at":          unixOrZero(link.UpdatedAt),
		"expires_at":          unixPtrOrZero(link.ExpiresAt),
		"is_deleted":          boolString(link.IsDeleted),
		"object_bucket":       link.ObjectBucket,
		"object_key":          link.ObjectKey,
		"object_file_name":    link.ObjectFileName,
		"object_content_type": link.ObjectContentType,
		"object_size":         link.ObjectSize,
	}
}

func linkFromHash(values map[string]string) shortener.Link {
	return shortener.Link{
		Code:              values["code"],
		Kind:              values["kind"],
		URL:               values["url"],
		Title:             values["title"],
		Scope:             values["scope"],
		Visits:            int64Field(values, "visits"),
		MaxVisits:         int64Field(values, "max_visits"),
		CreatedAt:         timeField(values, "created_at"),
		UpdatedAt:         timeField(values, "updated_at"),
		IsDeleted:         boolField(values, "is_deleted"),
		ExpiresAt:         timePtrField(values, "expires_at"),
		ObjectBucket:      values["object_bucket"],
		ObjectKey:         values["object_key"],
		ObjectFileName:    values["object_file_name"],
		ObjectContentType: values["object_content_type"],
		ObjectSize:        int64Field(values, "object_size"),
	}
}

func int64Field(values map[string]string, key string) int64 {
	parsed, _ := strconv.ParseInt(values[key], 10, 64)
	return parsed
}

func timeField(values map[string]string, key string) time.Time {
	unix := int64Field(values, key)
	if unix <= 0 {
		return time.Time{}
	}
	return time.Unix(unix, 0).UTC()
}

func timePtrField(values map[string]string, key string) *time.Time {
	unix := int64Field(values, key)
	if unix <= 0 {
		return nil
	}
	value := time.Unix(unix, 0).UTC()
	return &value
}

func boolField(values map[string]string, key string) bool {
	value := strings.ToLower(strings.TrimSpace(values[key]))
	return value == "1" || value == "true"
}

func unixOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func unixPtrOrZero(value *time.Time) int64 {
	if value == nil || value.IsZero() {
		return 0
	}
	return value.Unix()
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
