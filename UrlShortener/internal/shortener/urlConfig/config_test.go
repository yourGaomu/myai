package urlConfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigFile(t *testing.T) {
	path := writeConfig(t, `
server:
  addr: ":19081"
  base_url: "http://short.local"
  default_ttl_seconds: 120
store:
  type: "mongo"
mongo:
  uri: "mongodb://mongo.local:27017"
  database: "shortener"
  collection: "links"
redis:
  enabled: true
  addr: "redis.local:6379"
  password: "secret"
  db: 2
  prefix: "test:short"
  cache_ttl_seconds: 30
  lock_ttl_seconds: 4
  lock_wait_ms: 80
  visit_sync_interval_ms: 500
  visit_sync_queue_size: 128
minio:
  endpoint: "minio.local:9000"
  access_key: "ak"
  secret_key: "sk"
  bucket: "assets"
  use_ssl: true
  ensure_bucket: false
object:
  url_ttl_seconds: 900
`)

	config, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if config.Addr != ":19081" {
		t.Fatalf("unexpected addr: %s", config.Addr)
	}
	if config.BaseURL != "http://short.local" {
		t.Fatalf("unexpected base url: %s", config.BaseURL)
	}
	if config.DefaultTTL != 120*time.Second {
		t.Fatalf("unexpected default ttl: %s", config.DefaultTTL)
	}
	if config.StoreType != "mongo" {
		t.Fatalf("unexpected store type: %s", config.StoreType)
	}
	if config.MongoURI != "mongodb://mongo.local:27017" || config.MongoDatabase != "shortener" || config.MongoCollection != "links" {
		t.Fatalf("unexpected mongo config: %+v", config)
	}
	if !config.RedisEnabled || config.RedisAddr != "redis.local:6379" || config.RedisPassword != "secret" || config.RedisDB != 2 {
		t.Fatalf("unexpected redis config: %+v", config)
	}
	if config.RedisCacheTTL != 30*time.Second || config.RedisLockTTL != 4*time.Second || config.RedisLockWait != 80*time.Millisecond {
		t.Fatalf("unexpected redis ttl config: %+v", config)
	}
	if config.VisitSyncInterval != 500*time.Millisecond || config.VisitSyncQueueSize != 128 {
		t.Fatalf("unexpected visit sync config: %+v", config)
	}
	if config.MinIOEndpoint != "minio.local:9000" || config.MinIOAccessKey != "ak" || config.MinIOSecretKey != "sk" || config.MinIOBucket != "assets" {
		t.Fatalf("unexpected minio config: %+v", config)
	}
	if !config.MinIOUseSSL || config.MinIOEnsureBucket {
		t.Fatalf("unexpected minio bool config: %+v", config)
	}
	if config.ObjectURLTTL != 900*time.Second {
		t.Fatalf("unexpected object url ttl: %s", config.ObjectURLTTL)
	}
}

func TestEnvOverridesConfigFile(t *testing.T) {
	path := writeConfig(t, `
server:
  addr: ":19081"
store:
  type: "memory"
redis:
  enabled: false
`)
	t.Setenv("URL_SHORTENER_ADDR", ":28081")
	t.Setenv("URL_SHORTENER_STORE", "mongo")
	t.Setenv("URL_SHORTENER_REDIS_ENABLED", "true")

	config, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if config.Addr != ":28081" {
		t.Fatalf("env addr should override file: %s", config.Addr)
	}
	if config.StoreType != "mongo" {
		t.Fatalf("env store should override file: %s", config.StoreType)
	}
	if !config.RedisEnabled {
		t.Fatal("env redis enabled should override file")
	}
}

func TestLoadConfigFromEnvPath(t *testing.T) {
	path := writeConfig(t, `
server:
  addr: ":30081"
`)
	t.Setenv("URL_SHORTENER_CONFIG", path)

	config, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.Addr != ":30081" {
		t.Fatalf("unexpected addr: %s", config.Addr)
	}
}

func TestLoadDiscoversResourceApplicationYaml(t *testing.T) {
	current, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		if err := os.Chdir(current); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	if err := os.MkdirAll("resource", 0755); err != nil {
		t.Fatalf("mkdir resource: %v", err)
	}
	if err := os.WriteFile(filepath.Join("resource", "application.yaml"), []byte(`
server:
  addr: ":31081"
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.Addr != ":31081" {
		t.Fatalf("unexpected addr: %s", config.Addr)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "application.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
