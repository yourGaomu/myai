package execution

import "time"

type RunRequest struct {
	Command        string
	WorkDir        string
	Timeout        time.Duration
	MaxOutputBytes int
}

type RunResult struct {
	Command              string
	WorkDir              string
	ExitCode             int
	Stdout               string
	Stderr               string
	TimedOut             bool
	Truncated            bool
	DurationMS           int64
	ExecutionEnvironment string
	Isolated             bool
	Shell                string
	ErrorMessage         string
}
