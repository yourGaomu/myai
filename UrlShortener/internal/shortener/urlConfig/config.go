package urlConfig

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	envConfigPath = "URL_SHORTENER_CONFIG"
)

type Config struct {
	Addr               string
	BaseURL            string
	DefaultTTL         time.Duration
	StoreType          string
	MongoURI           string
	MongoDatabase      string
	MongoCollection    string
	RedisEnabled       bool
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	RedisPrefix        string
	RedisCacheTTL      time.Duration
	RedisLockTTL       time.Duration
	RedisLockWait      time.Duration
	VisitSyncInterval  time.Duration
	VisitSyncQueueSize int
	MinIOEndpoint      string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOBucket        string
	MinIOUseSSL        bool
	MinIOEnsureBucket  bool
	ObjectURLTTL       time.Duration
}

func ConfigFromEnv() Config {
	return configFromViper(newViper())
}

func Load(path string) (Config, error) {
	v := newViper()

	configPath := resolveConfigPath(path)
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, err
		}
	}

	return configFromViper(v), nil
}

func DefaultConfig() Config {
	addr := ":18081"
	return Config{
		Addr:               addr,
		BaseURL:            "http://localhost" + addr,
		DefaultTTL:         24 * time.Hour,
		StoreType:          "memory",
		MongoURI:           "mongodb://localhost:27017",
		MongoDatabase:      "myai",
		MongoCollection:    "short_links",
		RedisEnabled:       false,
		RedisAddr:          "localhost:6379",
		RedisPassword:      "",
		RedisDB:            0,
		RedisPrefix:        "myai:url-shortener",
		RedisCacheTTL:      10 * time.Minute,
		RedisLockTTL:       3 * time.Second,
		RedisLockWait:      120 * time.Millisecond,
		VisitSyncInterval:  time.Second,
		VisitSyncQueueSize: 4096,
		MinIOEndpoint:      "localhost:9000",
		MinIOAccessKey:     "",
		MinIOSecretKey:     "",
		MinIOBucket:        "myai-assets",
		MinIOUseSSL:        false,
		MinIOEnsureBucket:  true,
		ObjectURLTTL:       time.Hour,
	}
}

func newViper() *viper.Viper {
	v := viper.New()
	v.SetConfigType("yaml")

	bindEnv(v, "server.addr", "URL_SHORTENER_ADDR")
	bindEnv(v, "server.base_url", "URL_SHORTENER_BASE_URL")
	bindEnv(v, "server.default_ttl_seconds", "URL_SHORTENER_DEFAULT_TTL_SECONDS")
	bindEnv(v, "store.type", "URL_SHORTENER_STORE")
	bindEnv(v, "mongo.uri", "URL_SHORTENER_MONGO_URI")
	bindEnv(v, "mongo.database", "URL_SHORTENER_MONGO_DATABASE")
	bindEnv(v, "mongo.collection", "URL_SHORTENER_MONGO_COLLECTION")
	bindEnv(v, "redis.enabled", "URL_SHORTENER_REDIS_ENABLED")
	bindEnv(v, "redis.addr", "URL_SHORTENER_REDIS_ADDR")
	bindEnv(v, "redis.password", "URL_SHORTENER_REDIS_PASSWORD")
	bindEnv(v, "redis.db", "URL_SHORTENER_REDIS_DB")
	bindEnv(v, "redis.prefix", "URL_SHORTENER_REDIS_PREFIX")
	bindEnv(v, "redis.cache_ttl_seconds", "URL_SHORTENER_REDIS_CACHE_TTL_SECONDS")
	bindEnv(v, "redis.lock_ttl_seconds", "URL_SHORTENER_REDIS_LOCK_TTL_SECONDS")
	bindEnv(v, "redis.lock_wait_ms", "URL_SHORTENER_REDIS_LOCK_WAIT_MS")
	bindEnv(v, "redis.visit_sync_interval_ms", "URL_SHORTENER_VISIT_SYNC_INTERVAL_MS")
	bindEnv(v, "redis.visit_sync_queue_size", "URL_SHORTENER_VISIT_SYNC_QUEUE_SIZE")
	bindEnv(v, "minio.endpoint", "URL_SHORTENER_MINIO_ENDPOINT")
	bindEnv(v, "minio.access_key", "URL_SHORTENER_MINIO_ACCESS_KEY")
	bindEnv(v, "minio.secret_key", "URL_SHORTENER_MINIO_SECRET_KEY")
	bindEnv(v, "minio.bucket", "URL_SHORTENER_MINIO_BUCKET")
	bindEnv(v, "minio.use_ssl", "URL_SHORTENER_MINIO_USE_SSL")
	bindEnv(v, "minio.ensure_bucket", "URL_SHORTENER_MINIO_ENSURE_BUCKET")
	bindEnv(v, "object.url_ttl_seconds", "URL_SHORTENER_OBJECT_URL_TTL_SECONDS")

	return v
}

func bindEnv(v *viper.Viper, key string, env string) {
	if err := v.BindEnv(key, env); err != nil {
		panic(err)
	}
}

func resolveConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path != "" {
		return path
	}

	path = strings.TrimSpace(os.Getenv(envConfigPath))
	if path != "" {
		return path
	}

	return discoverConfigPath()
}

func discoverConfigPath() string {
	for _, path := range []string{
		"resource/application.yaml",
		"resource/application.yml",
	} {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func configFromViper(v *viper.Viper) Config {
	defaults := DefaultConfig()

	config := Config{
		Addr:               stringValue(v, "server.addr", defaults.Addr),
		BaseURL:            stringValue(v, "server.base_url", defaults.BaseURL),
		DefaultTTL:         secondsValue(v, "server.default_ttl_seconds", defaults.DefaultTTL),
		StoreType:          stringValue(v, "store.type", defaults.StoreType),
		MongoURI:           stringValue(v, "mongo.uri", defaults.MongoURI),
		MongoDatabase:      stringValue(v, "mongo.database", defaults.MongoDatabase),
		MongoCollection:    stringValue(v, "mongo.collection", defaults.MongoCollection),
		RedisEnabled:       boolValue(v, "redis.enabled", defaults.RedisEnabled),
		RedisAddr:          stringValue(v, "redis.addr", defaults.RedisAddr),
		RedisPassword:      stringValue(v, "redis.password", defaults.RedisPassword),
		RedisDB:            intValue(v, "redis.db", defaults.RedisDB),
		RedisPrefix:        stringValue(v, "redis.prefix", defaults.RedisPrefix),
		RedisCacheTTL:      secondsValue(v, "redis.cache_ttl_seconds", defaults.RedisCacheTTL),
		RedisLockTTL:       secondsValue(v, "redis.lock_ttl_seconds", defaults.RedisLockTTL),
		RedisLockWait:      millisecondsValue(v, "redis.lock_wait_ms", defaults.RedisLockWait),
		VisitSyncInterval:  millisecondsValue(v, "redis.visit_sync_interval_ms", defaults.VisitSyncInterval),
		VisitSyncQueueSize: intValue(v, "redis.visit_sync_queue_size", defaults.VisitSyncQueueSize),
		MinIOEndpoint:      stringValue(v, "minio.endpoint", defaults.MinIOEndpoint),
		MinIOAccessKey:     stringValue(v, "minio.access_key", defaults.MinIOAccessKey),
		MinIOSecretKey:     stringValue(v, "minio.secret_key", defaults.MinIOSecretKey),
		MinIOBucket:        stringValue(v, "minio.bucket", defaults.MinIOBucket),
		MinIOUseSSL:        boolValue(v, "minio.use_ssl", defaults.MinIOUseSSL),
		MinIOEnsureBucket:  boolValue(v, "minio.ensure_bucket", defaults.MinIOEnsureBucket),
		ObjectURLTTL:       secondsValue(v, "object.url_ttl_seconds", defaults.ObjectURLTTL),
	}

	config.StoreType = strings.ToLower(strings.TrimSpace(config.StoreType))
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = "http://localhost" + config.Addr
	}
	return config
}

func stringValue(v *viper.Viper, key string, fallback string) string {
	if !v.IsSet(key) {
		return fallback
	}
	return v.GetString(key)
}

func intValue(v *viper.Viper, key string, fallback int) int {
	if !v.IsSet(key) {
		return fallback
	}
	return v.GetInt(key)
}

func boolValue(v *viper.Viper, key string, fallback bool) bool {
	if !v.IsSet(key) {
		return fallback
	}
	return v.GetBool(key)
}

func secondsValue(v *viper.Viper, key string, fallback time.Duration) time.Duration {
	if !v.IsSet(key) {
		return fallback
	}
	value := v.GetInt64(key)
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}

func millisecondsValue(v *viper.Viper, key string, fallback time.Duration) time.Duration {
	if !v.IsSet(key) {
		return fallback
	}
	value := v.GetInt64(key)
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Millisecond
}
