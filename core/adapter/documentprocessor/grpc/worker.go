package grpcprocessor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "myai/core/adapter/documentprocessor/grpc/pb"
)

const workerTokenHeader = "x-myai-worker-token"

type worker struct {
	id         string
	token      string
	socketPath string
	client     pb.DocumentProcessorServiceClient
	connection *grpc.ClientConn
	command    *exec.Cmd
	exited     chan error
	closeOnce  sync.Once
	config     Config
}

func (worker *worker) context(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, workerTokenHeader, worker.token)
}

func (worker *worker) health(ctx context.Context) error {
	response, err := worker.client.Health(worker.context(ctx), &pb.HealthRequest{}, grpc.WaitForReady(true))
	if err != nil {
		return err
	}
	if response.GetStatus() != "ok" || response.GetProtocolVersion() != "v1" || response.GetWorkerId() != worker.id {
		return fmt.Errorf("unexpected health response from %s", worker.id)
	}
	return nil
}

func (worker *worker) Close() error {
	var closeErr error
	worker.closeOnce.Do(func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), worker.config.ShutdownGrace)
		defer cancel()
		_, _ = worker.client.Shutdown(worker.context(shutdownContext), &pb.ShutdownRequest{})
		if worker.connection != nil {
			closeErr = worker.connection.Close()
		}
		select {
		case <-worker.exited:
		case <-time.After(worker.config.ShutdownGrace):
			if worker.command != nil && worker.command.Process != nil {
				_ = worker.command.Process.Kill()
			}
			<-worker.exited
		}
		if worker.socketPath != "" {
			_ = os.Remove(worker.socketPath)
		}
	})
	return closeErr
}
