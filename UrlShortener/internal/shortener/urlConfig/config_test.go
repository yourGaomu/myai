package urlConfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFile(t *testing.T) {
	clearSecurityEnv(t)
	path := writeConfig(t, `
server:
  addr: ":19081"
  base_url: "http://short.local"
  api_key: "file-test-api-key"
  max_upload_bytes: 4096
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
  public_endpoint: "https://minio.example.com"
  region: "us-east-1"
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
	if config.APIKey != "file-test-api-key" || config.MaxUploadBytes != 4096 {
		t.Fatal("security settings were not loaded from YAML")
	}
	if err := config.Validate(); err != nil {
		t.Fatal("expected valid file security settings")
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
	if config.MinIOEndpoint != "minio.local:9000" || config.MinIOPublicEndpoint != "https://minio.example.com" || config.MinIORegion != "us-east-1" || config.MinIOAccessKey != "ak" || config.MinIOSecretKey != "sk" || config.MinIOBucket != "assets" {
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
  api_key: "file-test-api-key"
  max_upload_bytes: 4096
store:
  type: "memory"
redis:
  enabled: false
`)
	t.Setenv("URL_SHORTENER_ADDR", ":28081")
	t.Setenv("URL_SHORTENER_API_KEY", "env-test-api-key")
	t.Setenv("URL_SHORTENER_MAX_UPLOAD_BYTES", "8192")
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
	if config.APIKey != "env-test-api-key" || config.MaxUploadBytes != 8192 {
		t.Fatal("environment security settings should override YAML")
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

func TestSecurityConfigDefaults(t *testing.T) {
	clearSecurityEnv(t)
	config := ConfigFromEnv()
	if config.APIKey != "" || config.MaxUploadBytes != 20<<20 {
		t.Fatal("expected an empty default key and a bounded default upload size")
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "URL_SHORTENER_API_KEY") {
		t.Fatal("missing API key should fail validation")
	}
}

func TestEmptyAPIKeyEnvironmentOverridesFile(t *testing.T) {
	path := writeConfig(t, `
server:
  api_key: "file-test-api-key"
  max_upload_bytes: 4096
`)
	for _, value := range []string{"", " ", "\t\r\n"} {
		t.Setenv("URL_SHORTENER_API_KEY", value)
		config, err := Load(path)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		if config.APIKey != value || config.Validate() == nil {
			t.Fatal("empty or whitespace environment key must override YAML and fail validation")
		}
	}
}

func TestConfigRejectsInvalidUploadLimit(t *testing.T) {
	for _, value := range []string{"0", "-1", "invalid", "9223372036854775808"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("URL_SHORTENER_API_KEY", "config-test-api-key")
			t.Setenv("URL_SHORTENER_MAX_UPLOAD_BYTES", value)
			config := ConfigFromEnv()
			if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "URL_SHORTENER_MAX_UPLOAD_BYTES") {
				t.Fatal("invalid upload limit should fail validation")
			}
		})
	}
}

func TestConfigValidationDoesNotExposeKey(t *testing.T) {
	for _, key := range []string{"test-key with-space", "test-key\twith-tab", "test-key\r\nwith-newline"} {
		config := DefaultConfig()
		config.APIKey = key
		err := config.Validate()
		if err == nil || strings.Contains(err.Error(), "test-key") {
			t.Fatal("invalid API key must be rejected without exposing its value")
		}
	}
	config := DefaultConfig()
	config.APIKey = "config-test-api-key"
	if err := config.Validate(); err != nil {
		t.Fatal("expected valid security settings")
	}
}

func clearSecurityEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"URL_SHORTENER_API_KEY", "URL_SHORTENER_MAX_UPLOAD_BYTES"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset test environment variable: %v", err)
		}
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
