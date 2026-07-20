package grpcprocessor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "myai/core/adapter/documentprocessor/grpc/pb"
)

type workerSupervisor struct {
	config Config
}

type readyMessage struct {
	Event    string `json:"event"`
	Protocol string `json:"protocol"`
	WorkerID string `json:"worker_id"`
	Endpoint string `json:"endpoint"`
}

type readyResult struct {
	message readyMessage
	err     error
}

func newWorkerSupervisor(config Config) *workerSupervisor {
	return &workerSupervisor{config: config}
}

func (supervisor *workerSupervisor) Start(parent context.Context, index int) (*worker, error) {
	config := supervisor.config
	workerID := fmt.Sprintf("worker-%d-%s", index+1, uuid.NewString())
	token := uuid.NewString()
	address, socketPath, err := supervisor.address(workerID)
	if err != nil {
		return nil, err
	}
	script, err := filepath.Abs(config.Script)
	if err != nil {
		return nil, fmt.Errorf("resolve document processor script: %w", err)
	}
	args := []string{
		script,
		"worker",
		"--worker-id", workerID,
		"--address", address,
		"--max-input-bytes", strconv.FormatInt(config.MaxDocumentBytes, 10),
		"--grpc-max-message-bytes", strconv.Itoa(config.GRPCMaxMessageBytes),
		"--chunk-batch-size", strconv.Itoa(config.ChunkBatchSize),
		"--shutdown-grace-seconds", strconv.FormatFloat(config.ShutdownGrace.Seconds(), 'f', -1, 64),
	}
	if config.TempDir != "" {
		args = append(args, "--temp-dir", config.TempDir)
	}
	command := exec.Command(config.PythonExecutable, args...)
	if config.WorkingDir != "" {
		command.Dir = config.WorkingDir
	}
	command.Env = append(os.Environ(), "MYAI_DOCUMENT_PROCESSOR_TOKEN="+token)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open document processor stdout: %w", err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start document processor %s: %w", workerID, err)
	}

	exited := make(chan error, 1)
	go func() {
		exited <- command.Wait()
		close(exited)
	}()
	ready := make(chan readyResult, 1)
	go readWorkerReady(stdout, ready)

	startupContext, cancel := context.WithTimeout(parent, config.StartupTimeout)
	defer cancel()
	var message readyMessage
	select {
	case result := <-ready:
		if result.err != nil {
			_ = command.Process.Kill()
			return nil, fmt.Errorf("read document processor readiness: %w", result.err)
		}
		message = result.message
	case exitErr := <-exited:
		return nil, fmt.Errorf("document processor %s exited before readiness: %w", workerID, exitErr)
	case <-startupContext.Done():
		_ = command.Process.Kill()
		return nil, fmt.Errorf("wait for document processor %s readiness: %w", workerID, startupContext.Err())
	}
	if message.Event != "ready" || message.Protocol != "v1" || message.WorkerID != workerID || strings.TrimSpace(message.Endpoint) == "" {
		_ = command.Process.Kill()
		return nil, fmt.Errorf("document processor %s returned invalid readiness message", workerID)
	}

	connection, err := grpc.NewClient(
		message.Endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(config.GRPCMaxMessageBytes),
			grpc.MaxCallSendMsgSize(config.GRPCMaxMessageBytes),
		),
	)
	if err != nil {
		_ = command.Process.Kill()
		return nil, fmt.Errorf("create document processor connection: %w", err)
	}
	result := &worker{
		id:         workerID,
		token:      token,
		socketPath: socketPath,
		client:     pb.NewDocumentProcessorServiceClient(connection),
		connection: connection,
		command:    command,
		exited:     exited,
		config:     config,
	}
	if err := result.health(startupContext); err != nil {
		_ = result.Close()
		return nil, fmt.Errorf("health check document processor %s: %w", workerID, err)
	}
	return result, nil
}

func readWorkerReady(stdout io.Reader, ready chan<- readyResult) {
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		ready <- readyResult{err: err}
		return
	}
	var message readyMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &message); err != nil {
		ready <- readyResult{err: err}
		return
	}
	ready <- readyResult{message: message}
}

func (supervisor *workerSupervisor) address(workerID string) (string, string, error) {
	transport := supervisor.config.Transport
	if transport == TransportAuto {
		if runtime.GOOS == "windows" {
			transport = TransportTCP
		} else {
			transport = TransportUnix
		}
	}
	if transport == TransportTCP {
		return "127.0.0.1:0", "", nil
	}
	directory := supervisor.config.TempDir
	if directory == "" {
		directory = os.TempDir()
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", fmt.Errorf("create document processor socket directory: %w", err)
	}
	socketPath := filepath.Join(directory, "myai-document-"+workerID+".sock")
	_ = os.Remove(socketPath)
	return "unix:///" + filepath.ToSlash(socketPath), socketPath, nil
}
