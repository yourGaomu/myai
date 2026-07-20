package grpcprocessor

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

const (
	TransportAuto = "auto"
	TransportTCP  = "tcp"
	TransportUnix = "unix"
)

type Config struct {
	PythonExecutable    string
	Script              string
	WorkingDir          string
	Transport           string
	WorkerCount         int
	MaxPendingJobs      int
	StartupTimeout      time.Duration
	RequestTimeout      time.Duration
	ShutdownGrace       time.Duration
	MaxDocumentBytes    int64
	ContentChunkBytes   int
	ChunkBatchSize      int
	GRPCMaxMessageBytes int
	TempDir             string
}

func (config Config) Normalize() Config {
	config.PythonExecutable = strings.TrimSpace(config.PythonExecutable)
	if config.PythonExecutable == "" {
		config.PythonExecutable = "python"
	}
	config.Script = strings.TrimSpace(config.Script)
	config.WorkingDir = strings.TrimSpace(config.WorkingDir)
	config.Transport = strings.ToLower(strings.TrimSpace(config.Transport))
	if config.Transport == "" {
		config.Transport = TransportAuto
	}
	if config.WorkerCount == 0 {
		config.WorkerCount = min(4, max(1, runtime.NumCPU()-1))
	}
	if config.MaxPendingJobs == 0 {
		config.MaxPendingJobs = config.WorkerCount * 8
	}
	if config.StartupTimeout == 0 {
		config.StartupTimeout = 30 * time.Second
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 120 * time.Second
	}
	if config.ShutdownGrace == 0 {
		config.ShutdownGrace = 5 * time.Second
	}
	if config.MaxDocumentBytes == 0 {
		config.MaxDocumentBytes = 512 * 1024 * 1024
	}
	if config.ContentChunkBytes == 0 {
		config.ContentChunkBytes = 256 * 1024
	}
	if config.ChunkBatchSize == 0 {
		config.ChunkBatchSize = 64
	}
	if config.GRPCMaxMessageBytes == 0 {
		config.GRPCMaxMessageBytes = 8 * 1024 * 1024
	}
	config.TempDir = strings.TrimSpace(config.TempDir)
	return config
}

func (config Config) Validate() error {
	config = config.Normalize()
	if config.Script == "" {
		return fmt.Errorf("document processor Python script is required")
	}
	if config.Transport != TransportAuto && config.Transport != TransportTCP && config.Transport != TransportUnix {
		return fmt.Errorf("unsupported document processor transport %q", config.Transport)
	}
	if config.Transport == TransportUnix && runtime.GOOS == "windows" {
		return fmt.Errorf("unix document processor transport is not supported on Windows")
	}
	if config.WorkerCount < 1 {
		return fmt.Errorf("document processor worker count must be positive")
	}
	if config.MaxPendingJobs < 0 {
		return fmt.Errorf("document processor max pending jobs must not be negative")
	}
	if config.StartupTimeout <= 0 || config.RequestTimeout <= 0 || config.ShutdownGrace <= 0 {
		return fmt.Errorf("document processor timeouts must be positive")
	}
	if config.MaxDocumentBytes < 1 || config.ContentChunkBytes < 1 || config.ChunkBatchSize < 1 || config.GRPCMaxMessageBytes < 1 {
		return fmt.Errorf("document processor size limits must be positive")
	}
	if config.GRPCMaxMessageBytes < 128*1024 {
		return fmt.Errorf("document processor gRPC message size must be at least 128 KiB")
	}
	if config.ContentChunkBytes >= config.GRPCMaxMessageBytes {
		return fmt.Errorf("document content chunk size exceeds gRPC message size")
	}
	return nil
}
