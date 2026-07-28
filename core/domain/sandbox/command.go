package sandbox

import (
	"fmt"
	"strings"
	"time"
)

type CommandRequest struct {
	Command string
	WorkDir string
	Timeout time.Duration
	Env     map[string]string
}

func (request CommandRequest) Validate() error {
	if strings.TrimSpace(request.Command) == "" {
		return fmt.Errorf("sandbox command is required")
	}
	if request.Timeout < 0 {
		return fmt.Errorf("sandbox command timeout must not be negative")
	}
	return nil
}

type CommandResult struct {
	ExecutionID string
	Stdout      string
	Stderr      string
	ExitCode    int
	TimedOut    bool
	ErrorName   string
	ErrorValue  string
}
