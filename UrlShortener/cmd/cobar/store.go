package cobar

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"myai-url-shortener/internal/shortener/objectstore"
	"myai-url-shortener/internal/shortener/store"
	"myai-url-shortener/internal/shortener/store/cacheStore"
	"myai-url-shortener/internal/shortener/store/memoryStore"
	"myai-url-shortener/internal/shortener/store/mongoStore"
	"myai-url-shortener/internal/shortener/urlConfig"

	"github.com/redis/go-redis/v9"
)

func newObjectStore(ctx context.Context, config urlConfig.Config) (objectstore.Store, error) {
	if config.MinIOAccessKey == "" || config.MinIOSecretKey == "" {
		log.Println("url shortener object store: disabled")
		return nil, nil
	}

	minioStore, err := objectstore.NewMinIOStore(ctx, objectstore.MinIOOptions{
		Endpoint:        config.MinIOEndpoint,
		AccessKeyID:     config.MinIOAccessKey,
		SecretAccessKey: config.MinIOSecretKey,
		Bucket:          config.MinIOBucket,
		UseSSL:          config.MinIOUseSSL,
		EnsureBucket:    config.MinIOEnsureBucket,
	})
	if err != nil {
		return nil, err
	}
	log.Printf("url shortener object store: minio endpoint=%s bucket=%s", config.MinIOEndpoint, config.MinIOBucket)
	return minioStore, nil
}

func newLinkStore(ctx context.Context, config urlConfig.Config) (store.Store, func(), error) {
	var linkStore store.Store
	cleanup := func() {}

	switch strings.ToLower(config.StoreType) {
	case "", "memory":
		log.Println("url shortener store: memory")
		linkStore = memoryStore.NewMemoryStore()
	case "mongo", "mongodb":
		mongoLinkStore, err := mongoStore.NewMongoStore(
			ctx,
			config.MongoURI,
			config.MongoDatabase,
			config.MongoCollection,
		)
		if err != nil {
			return nil, nil, err
		}
		log.Printf("url shortener store: mongo database=%s collection=%s", config.MongoDatabase, config.MongoCollection)
		linkStore = mongoLinkStore
		cleanup = func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := mongoLinkStore.Close(closeCtx); err != nil {
				log.Printf("close mongo store failed: %v", err)
			}
		}
	default:
		return nil, nil, fmt.Errorf("unsupported store type: %s", config.StoreType)
	}

	if !config.RedisEnabled {
		return linkStore, cleanup, nil
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		return nil, nil, err
	}

	previousCleanup := cleanup
	cleanup = func() {
		previousCleanup()
		if err := redisClient.Close(); err != nil {
			log.Printf("close redis cache failed: %v", err)
		}
	}
	log.Printf("url shortener cache: redis addr=%s db=%d ttl=%s", config.RedisAddr, config.RedisDB, config.RedisCacheTTL)
	cachedStore := cacheStore.NewCachedStore(linkStore, redisClient, cacheStore.Options{
		Prefix:             config.RedisPrefix,
		TTL:                config.RedisCacheTTL,
		LockTTL:            config.RedisLockTTL,
		LockWait:           config.RedisLockWait,
		VisitSyncInterval:  config.VisitSyncInterval,
		VisitSyncQueueSize: config.VisitSyncQueueSize,
	})

	previousCleanup = cleanup
	cleanup = func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cachedStore.Close(closeCtx); err != nil {
			log.Printf("close visit syncer failed: %v", err)
		}
		previousCleanup()
	}
	return cachedStore, cleanup, nil
}
