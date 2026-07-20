package milvus

import (
	"fmt"
	"strings"
)

const defaultCollectionPrefix = "knowledge_vectors"

type Config struct {
	Address          string
	Username         string
	Password         string
	Database         string
	APIKey           string
	EnableTLS        bool
	CollectionPrefix string
	Shards           int32
}

func (config Config) normalize() Config {
	config.Address = strings.TrimSpace(config.Address)
	config.Username = strings.TrimSpace(config.Username)
	config.Database = strings.TrimSpace(config.Database)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.CollectionPrefix = strings.TrimSpace(config.CollectionPrefix)
	if config.CollectionPrefix == "" {
		config.CollectionPrefix = defaultCollectionPrefix
	}
	if config.Shards == 0 {
		config.Shards = 1
	}
	return config
}

func (config Config) validate() error {
	if config.Address == "" {
		return fmt.Errorf("Milvus address is required")
	}
	if config.Shards < 1 {
		return fmt.Errorf("Milvus shards must be positive")
	}
	return nil
}
