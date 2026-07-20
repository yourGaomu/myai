package openaicompatible

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultBatchSize = 32
	defaultTimeout   = 60 * time.Second
)

type Config struct {
	APIKey            string
	BaseURL           string
	Model             string
	Dimensions        int
	BatchSize         int
	Timeout           time.Duration
	RequestDimensions bool
	PreserveNewLines  bool
}

func (config Config) normalize() Config {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.Model = strings.TrimSpace(config.Model)
	if config.BatchSize == 0 {
		config.BatchSize = defaultBatchSize
	}
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}
	return config
}

func (config Config) validate() error {
	if config.APIKey == "" {
		return fmt.Errorf("embedding API key is required")
	}
	if config.Model == "" {
		return fmt.Errorf("embedding model is required")
	}
	if config.Dimensions < 1 {
		return fmt.Errorf("embedding dimensions must be positive")
	}
	if config.BatchSize < 1 {
		return fmt.Errorf("embedding batch size must be positive")
	}
	if config.Timeout < 0 {
		return fmt.Errorf("embedding timeout must not be negative")
	}
	return nil
}
