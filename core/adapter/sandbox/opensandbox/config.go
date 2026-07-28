package opensandbox

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	sdk "github.com/alibaba/OpenSandbox/sdks/sandbox/go"
)

const (
	defaultImage          = "python:3.12-slim"
	defaultCPU            = "500m"
	defaultMemory         = "512Mi"
	defaultSandboxTimeout = 5 * time.Minute
	defaultCommandTimeout = time.Minute
	defaultRequestTimeout = 30 * time.Second
	defaultDownloadLimit  = int64(16 * 1024 * 1024)
)

type Config struct {
	Endpoint         string
	APIKey           string
	UseServerProxy   bool
	Image            string
	CPU              string
	Memory           string
	SandboxTimeout   time.Duration
	CommandTimeout   time.Duration
	RequestTimeout   time.Duration
	MaxDownloadBytes int64
}

func (config Config) normalize() Config {
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Image = strings.TrimSpace(config.Image)
	config.CPU = strings.TrimSpace(config.CPU)
	config.Memory = strings.TrimSpace(config.Memory)
	if config.Image == "" {
		config.Image = defaultImage
	}
	if config.CPU == "" {
		config.CPU = defaultCPU
	}
	if config.Memory == "" {
		config.Memory = defaultMemory
	}
	if config.SandboxTimeout <= 0 {
		config.SandboxTimeout = defaultSandboxTimeout
	}
	if config.CommandTimeout <= 0 {
		config.CommandTimeout = defaultCommandTimeout
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaultRequestTimeout
	}
	if config.MaxDownloadBytes <= 0 {
		config.MaxDownloadBytes = defaultDownloadLimit
	}
	return config
}

func (config Config) validate() error {
	if config.Endpoint == "" {
		return fmt.Errorf("OpenSandbox endpoint is required")
	}
	if config.APIKey == "" {
		return fmt.Errorf("OpenSandbox API key is required")
	}
	if config.SandboxTimeout < time.Minute {
		return fmt.Errorf("OpenSandbox timeout must be at least one minute")
	}
	if config.CommandTimeout <= 0 {
		return fmt.Errorf("OpenSandbox command timeout must be positive")
	}
	if config.MaxDownloadBytes <= 0 {
		return fmt.Errorf("OpenSandbox download limit must be positive")
	}
	return nil
}

func (config Config) connectionConfig() (sdk.ConnectionConfig, error) {
	endpoint := config.Endpoint
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return sdk.ConnectionConfig{}, fmt.Errorf("parse OpenSandbox endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return sdk.ConnectionConfig{}, fmt.Errorf("unsupported OpenSandbox protocol %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return sdk.ConnectionConfig{}, fmt.Errorf("OpenSandbox endpoint host is required")
	}
	path := strings.Trim(parsed.Path, "/")
	if path != "" && path != "v1" {
		return sdk.ConnectionConfig{}, fmt.Errorf("OpenSandbox endpoint path must be empty or /v1")
	}
	return sdk.ConnectionConfig{
		Domain:         parsed.Host,
		Protocol:       parsed.Scheme,
		APIKey:         config.APIKey,
		UseServerProxy: config.UseServerProxy,
		RequestTimeout: config.RequestTimeout,
		Retry:          retryConfig(),
	}, nil
}

func retryConfig() *sdk.RetryConfig {
	config := sdk.DefaultRetryConfig()
	return &config
}
