package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/gorilla/websocket"

	"myai/core/port/files"
	"myai/core/remote/protocol"
)

const operationQueueLimit = 64
const asyncOperationLimit = 32

type queuedOperation struct {
	ctx context.Context
	run func()
}

// One worker preserves receive order without one goroutine per queued request.
// Admission never waits on a slow facade, so control messages remain readable.
type operationQueue struct {
	mu      sync.Mutex
	pending []queuedOperation
	running bool
	idle    chan struct{}
}

func (q *operationQueue) enqueue(ctx context.Context, run func()) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(q.pending) >= operationQueueLimit {
		return errors.New("remote operation queue is full; retry later")
	}
	q.pending = append(q.pending, queuedOperation{ctx: ctx, run: run})
	if !q.running {
		q.running = true
		q.idle = make(chan struct{})
		go q.work()
	}
	return nil
}

func (q *operationQueue) work() {
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.running = false
			close(q.idle)
			q.mu.Unlock()
			return
		}
		item := q.pending[0]
		q.pending[0] = queuedOperation{}
		q.pending = q.pending[1:]
		q.mu.Unlock()
		if item.ctx.Err() == nil {
			item.run()
		}
	}
}

func (q *operationQueue) wait(ctx context.Context) error {
	q.mu.Lock()
	idle := q.idle
	q.mu.Unlock()
	if idle == nil {
		return nil
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Agent) enqueueOperation(ctx context.Context, conn *websocket.Conn, message protocol.Message, handle func(context.Context, *websocket.Conn, protocol.Message) error) error {
	return a.operations.enqueue(ctx, func() {
		if err := handle(ctx, conn, message); err != nil {
			if a.writeHandlerError(conn, message, err) != nil {
				_ = conn.Close()
			}
		}
	})
}

func (a *Agent) dispatchRelayMessage(ctx, connectionCtx context.Context, conn *websocket.Conn, message protocol.Message) error {
	switch message.Type {
	case protocol.TypeHeartbeat:
		return nil
	case protocol.TypePermissionResult, protocol.TypeUserQuestionAnswer, protocol.TypeSessionPause:
		return a.handleRelayMessage(connectionCtx, conn, message)
	case protocol.TypeUserMessage, protocol.TypeSessionRegenerate, protocol.TypeSessionPlanExecute, protocol.TypeAIMemoryDreamRun, protocol.TypeSubagentTaskResume:
		return a.handleRelayMessage(ctx, conn, message)
	case protocol.TypeAssetUploadBegin, protocol.TypeAssetUploadChunk, protocol.TypeAssetUploadStatus, protocol.TypeAssetUploadComplete, protocol.TypeAssetUploadCancel:
		return a.enqueueUploadOperation(connectionCtx, conn, message)
	case protocol.TypeAssetImagePreview:
		return a.startAssetImagePreview(connectionCtx, conn, message)
	case protocol.TypeSubagentTaskWait:
		return a.handleRelayMessage(connectionCtx, conn, message)
	default:
		return a.enqueueOperation(connectionCtx, conn, message, a.handleRelayMessage)
	}
}

func (a *Agent) waitForOperations(ctx context.Context) error {
	// Queued handlers may start tracked completion/reply workers. Join queue
	// admission first so tasks.Wait cannot race a new Add from an idle queue.
	if err := a.operations.wait(ctx); err != nil {
		return err
	}
	if err := a.catalogOperations.wait(ctx); err != nil {
		return err
	}
	if err := a.uploadOperations.wait(ctx); err != nil {
		return err
	}
	joined := make(chan struct{})
	go func() { a.tasks.Wait(); close(joined) }()
	select {
	case <-joined:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Agent) startAsyncOperation(ctx context.Context, conn *websocket.Conn, message protocol.Message, handle func(context.Context, *websocket.Conn, protocol.Message)) error {
	if a.activeOperations.Add(1) > asyncOperationLimit {
		a.activeOperations.Add(-1)
		return errors.New("too many active remote operations; retry later")
	}
	a.tasks.Add(1)
	go func() {
		defer a.tasks.Done()
		defer a.activeOperations.Add(-1)
		handle(ctx, conn, message)
	}()
	return nil
}

// Control replies reuse the chat request ID. Tag their errors so neither Relay
// nor a client mistakes a failed approval for a terminal chat failure.
func (a *Agent) writeHandlerError(conn *websocket.Conn, message protocol.Message, err error) error {
	if message.Type == protocol.TypeMCPDetail {
		return a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, protocol.MCPDetailErrorPayload{
			ErrorPayload: protocol.ErrorPayload{Message: err.Error()}, RequestType: protocol.TypeMCPDetail,
		})
	}
	payload := struct {
		protocol.ErrorPayload
		RequestType protocol.MessageType `json:"request_type,omitempty"`
		ApprovalID  string               `json:"approval_id,omitempty"`
		QuestionID  string               `json:"question_id,omitempty"`
	}{ErrorPayload: protocol.ErrorPayload{Message: err.Error()}, RequestType: message.Type}
	if protocol.IsAssetUploadRequest(message.Type) {
		switch {
		case errors.Is(err, files.ErrUploadServiceUnavailable):
			payload.ErrorPayload = protocol.ErrorPayload{Message: files.ErrUploadServiceUnavailable.Error(), Code: protocol.UploadErrorServiceUnavailable}
		case errors.Is(err, files.ErrUploadMissingTool):
			payload.ErrorPayload = protocol.ErrorPayload{Message: files.ErrUploadMissingTool.Error(), Code: protocol.UploadErrorMissingTool}
		case errors.Is(err, files.ErrUploadUpstreamFailure):
			payload.ErrorPayload = protocol.ErrorPayload{Message: files.ErrUploadUpstreamFailure.Error(), Code: protocol.UploadErrorUpstreamFailure}
		}
		return a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, payload)
	}
	var identity struct {
		ApprovalID string `json:"approval_id"`
		QuestionID string `json:"question_id"`
	}
	_ = json.Unmarshal(message.Payload, &identity)
	payload.ApprovalID, payload.QuestionID = identity.ApprovalID, identity.QuestionID
	return a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, payload)
}
