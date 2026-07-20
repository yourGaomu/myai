package minioadapter

import (
	"fmt"
	"strings"
)

type Config struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	SessionToken     string
	Bucket           string
	Region           string
	UseSSL           bool
	AutoCreateBucket bool
}

func (config Config) Validate() error {
	if strings.TrimSpace(config.Endpoint) == "" {
		return fmt.Errorf("minio endpoint is required")
	}
	if strings.TrimSpace(config.AccessKey) == "" {
		return fmt.Errorf("minio access key is required")
	}
	if strings.TrimSpace(config.SecretKey) == "" {
		return fmt.Errorf("minio secret key is required")
	}
	if strings.TrimSpace(config.Bucket) == "" {
		return fmt.Errorf("minio bucket is required")
	}
	return nil
}
