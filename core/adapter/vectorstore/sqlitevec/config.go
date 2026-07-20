package sqlitevec

import (
	"fmt"
	"strings"
)

const defaultMaxOpenConnections = 4

type Config struct {
	Path               string
	MaxOpenConnections int
}

func (config Config) normalize() Config {
	config.Path = strings.TrimSpace(config.Path)
	if config.MaxOpenConnections == 0 {
		config.MaxOpenConnections = defaultMaxOpenConnections
	}
	return config
}

func (config Config) validate() error {
	if config.Path == "" {
		return fmt.Errorf("sqlite-vec path is required")
	}
	if config.MaxOpenConnections < 1 {
		return fmt.Errorf("sqlite-vec max open connections must be positive")
	}
	return nil
}
