package sandbox

import (
	"fmt"
	"strings"
	"time"
)

type CreateRequest struct {
	Image         string
	Entrypoint    []string
	Timeout       time.Duration
	CPU           string
	Memory        string
	Env           map[string]string
	Metadata      map[string]string
	NetworkPolicy *NetworkPolicy
}

func (request CreateRequest) Validate() error {
	if request.Timeout < 0 {
		return fmt.Errorf("sandbox timeout must not be negative")
	}
	if request.NetworkPolicy != nil {
		if err := request.NetworkPolicy.Validate(); err != nil {
			return err
		}
	}
	for key := range request.Env {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("sandbox environment variable name is required")
		}
	}
	return nil
}
