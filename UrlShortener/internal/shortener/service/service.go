package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/objectstore"
	"myai-url-shortener/internal/shortener/store"
	"net/url"
	"strings"
	"time"
)

const (
	defaultCodeLength = 8
	maxCreateRetries  = 8
)

var (
	ErrInvalidURL      = errors.New("url is invalid")
	ErrLinkExpired     = errors.New("link is expired")
	ErrVisitsExhausted = errors.New("link max visits exhausted")
)

type ServiceOptions struct {
	BaseURL      string
	DefaultTTL   time.Duration
	ObjectStore  objectstore.Store
	ObjectURLTTL time.Duration
}

type Service struct {
	store        store.Store
	objectStore  objectstore.Store
	baseURL      string
	defaultTTL   time.Duration
	objectURLTTL time.Duration
	now          func() time.Time
}

type CreateObjectLinkRequest struct {
	Reader      io.Reader
	FileName    string
	ContentType string
	Size        int64
	Title       string
	Scope       string
	TTLSeconds  int64
	MaxVisits   int64
}

func (s *Service) GetLinkInfo(ctx context.Context, code string) (shortener.Link, error) {
	if s.store == nil {
		return shortener.Link{}, errors.New("store is nil")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return shortener.Link{}, store.ErrLinkNotFound
	}
	return s.store.Get(ctx, code)
}

func (s *Service) ListLinks(ctx context.Context) ([]shortener.Link, error) {
	if s.store == nil {
		return nil, errors.New("store is nil")
	}

	links, err := s.store.List(ctx)

	if err != nil {
		return make([]shortener.Link, 0), err
	}

	return links, nil
}

func NewService(store store.Store, options ServiceOptions) *Service {
	baseURL := strings.TrimRight(options.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:18081"
	}
	defaultTTL := options.DefaultTTL
	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour
	}
	objectURLTTL := options.ObjectURLTTL
	if objectURLTTL <= 0 {
		objectURLTTL = time.Hour
	}

	return &Service{
		store:        store,
		objectStore:  options.ObjectStore,
		baseURL:      baseURL,
		defaultTTL:   defaultTTL,
		objectURLTTL: objectURLTTL,
		now:          time.Now,
	}
}

func (s *Service) CreateLink(ctx context.Context, request shortener.CreateLinkRequest) (shortener.CreateLinkResponse, error) {
	if s.store == nil {
		return shortener.CreateLinkResponse{}, errors.New("store is nil")
	}

	targetURL, err := normalizeURL(request.URL)
	if err != nil {
		return shortener.CreateLinkResponse{}, err
	}

	ttl := s.defaultTTL
	if request.TTLSeconds > 0 {
		ttl = time.Duration(request.TTLSeconds) * time.Second
	}
	expiresAt := s.now().Add(ttl)

	var code string
	for attempt := 0; attempt < maxCreateRetries; attempt++ {
		code, err = randomCode(defaultCodeLength)
		if err != nil {
			return shortener.CreateLinkResponse{}, err
		}
		now := s.now()
		link := shortener.Link{
			Code:      code,
			Kind:      shortener.LinkKindURL,
			URL:       targetURL,
			Title:     strings.TrimSpace(request.Title),
			Scope:     strings.TrimSpace(request.Scope),
			MaxVisits: request.MaxVisits,
			CreatedAt: now,
			UpdatedAt: now,
			ExpiresAt: &expiresAt,
		}
		err = s.store.Create(ctx, link)
		if err == nil {
			return shortener.CreateLinkResponse{
				Code:      code,
				ShortURL:  s.ShortURL(code),
				URL:       targetURL,
				ExpiresAt: &expiresAt,
			}, nil
		}
		if !errors.Is(err, store.ErrCodeExists) {
			return shortener.CreateLinkResponse{}, err
		}
	}

	return shortener.CreateLinkResponse{}, fmt.Errorf("generate unique code failed after %d attempts", maxCreateRetries)
}

func (s *Service) CreateObjectLink(ctx context.Context, request CreateObjectLinkRequest) (shortener.CreateObjectLinkResponse, error) {
	if s.store == nil {
		return shortener.CreateObjectLinkResponse{}, errors.New("store is nil")
	}
	if s.objectStore == nil {
		return shortener.CreateObjectLinkResponse{}, errors.New("object store is nil")
	}

	objectInfo, err := s.objectStore.Upload(ctx, objectstore.UploadRequest{
		Reader:      request.Reader,
		FileName:    request.FileName,
		ContentType: request.ContentType,
		Size:        request.Size,
	})
	if err != nil {
		return shortener.CreateObjectLinkResponse{}, err
	}

	ttl := s.defaultTTL
	if request.TTLSeconds > 0 {
		ttl = time.Duration(request.TTLSeconds) * time.Second
	}
	expiresAt := s.now().Add(ttl)

	var code string
	for attempt := 0; attempt < maxCreateRetries; attempt++ {
		code, err = randomCode(defaultCodeLength)
		if err != nil {
			return shortener.CreateObjectLinkResponse{}, err
		}
		now := s.now()
		link := shortener.Link{
			Code:              code,
			Kind:              shortener.LinkKindObject,
			Title:             strings.TrimSpace(request.Title),
			Scope:             strings.TrimSpace(request.Scope),
			MaxVisits:         request.MaxVisits,
			CreatedAt:         now,
			UpdatedAt:         now,
			ExpiresAt:         &expiresAt,
			ObjectBucket:      objectInfo.Bucket,
			ObjectKey:         objectInfo.Key,
			ObjectFileName:    objectInfo.FileName,
			ObjectContentType: objectInfo.ContentType,
			ObjectSize:        objectInfo.Size,
		}
		err = s.store.Create(ctx, link)
		if err == nil {
			return shortener.CreateObjectLinkResponse{
				Code:        code,
				ShortURL:    s.ShortURL(code),
				Bucket:      objectInfo.Bucket,
				ObjectKey:   objectInfo.Key,
				FileName:    objectInfo.FileName,
				ContentType: objectInfo.ContentType,
				Size:        objectInfo.Size,
				ExpiresAt:   &expiresAt,
			}, nil
		}
		if !errors.Is(err, store.ErrCodeExists) {
			return shortener.CreateObjectLinkResponse{}, err
		}
	}

	return shortener.CreateObjectLinkResponse{}, fmt.Errorf("generate unique code failed after %d attempts", maxCreateRetries)
}

func (s *Service) Resolve(ctx context.Context, code string) (shortener.Link, error) {
	if s.store == nil {
		return shortener.Link{}, errors.New("store is nil")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return shortener.Link{}, store.ErrLinkNotFound
	}

	link, err := s.store.Get(ctx, code)
	if err != nil {
		return shortener.Link{}, err
	}
	if link.Expired(s.now()) {
		return shortener.Link{}, ErrLinkExpired
	}
	if link.VisitsExhausted() {
		return shortener.Link{}, ErrVisitsExhausted
	}
	link, err = s.store.IncrementVisits(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrLinkExpired) {
			return shortener.Link{}, ErrLinkExpired
		}
		if errors.Is(err, store.ErrVisitsExhausted) {
			return shortener.Link{}, ErrVisitsExhausted
		}
		return shortener.Link{}, err
	}
	if link.Kind == shortener.LinkKindObject {
		if s.objectStore == nil {
			return shortener.Link{}, errors.New("object store is nil")
		}
		signedURL, err := s.objectStore.PresignedGetURL(ctx, link.ObjectBucket, link.ObjectKey, s.objectURLTTL)
		if err != nil {
			return shortener.Link{}, err
		}
		link.URL = signedURL
	}
	return link, nil
}

func (s *Service) ShortURL(code string) string {
	return s.baseURL + "/s/" + code
}

func normalizeURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidURL
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidURL
	}
	return parsed.String(), nil
}

func randomCode(length int) (string, error) {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	if length <= 0 {
		length = defaultCodeLength
	}

	var builder strings.Builder
	builder.Grow(length)
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < length; i++ {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(alphabet[index.Int64()])
	}
	return builder.String(), nil
}

func (s *Service) DeleteLinkInfo(ctx context.Context, code string) (shortener.Link, error) {
	if s.store == nil {
		return shortener.Link{}, errors.New("store is nil")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return shortener.Link{}, store.ErrLinkNotFound
	}

	link, err := s.store.Delete(ctx, code)
	if err != nil {
		return shortener.Link{}, err
	}
	return link, nil
}
