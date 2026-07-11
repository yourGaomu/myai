package chat

import (
	"context"
	"errors"
	"testing"

	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestGenerationTaskServiceWrapsGenerationWithRecorder(t *testing.T) {
	recorder := &taskRecorderFake{}
	factory := &taskRecorderFactoryFake{recorder: recorder}
	generator := &taskGenerationHandlerFake{
		result: GenerationResponse{
			SessionID: "session-1",
			Result:    modelport.ChatResult{Content: "answer"},
		},
	}

	response, err := GenerationTaskService{
		RequestIDs: taskRequestIDsFake{id: "request-1"},
		Recorders:  factory,
		Generator:  generator,
	}.Generate(context.Background(), GenerationTaskCommand{
		Session:       taskSession(),
		LatestInput:   "hello",
		Title:         "title",
		Reason:        "reason",
		CapturePlan:   true,
		ForceChatMode: true,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if response.Result.Content != "answer" {
		t.Fatalf("expected generation response, got %#v", response)
	}
	if factory.record.Title != "title" || factory.record.Reason != "reason" || factory.record.SessionID != "session-1" || factory.record.RequestID != "request-1" {
		t.Fatalf("expected task record metadata, got %#v", factory.record)
	}
	if generator.command.RequestID != "request-1" || generator.command.LatestInput != "hello" || !generator.command.CapturePlan || !generator.command.ForceChatMode {
		t.Fatalf("expected generation command to be forwarded, got %#v", generator.command)
	}
	if attached, _ := generator.ctx.Value(taskRecorderAttachedKey{}).(bool); !attached {
		t.Fatal("expected recorder to attach to generation context")
	}
	if got := recorder.events; len(got) != 3 || got[0] != "attach" || got[1] != "save" || got[2] != "close" {
		t.Fatalf("expected attach/save/close order, got %#v", got)
	}
}

func TestGenerationTaskServiceSavesAndClosesWhenGenerationFails(t *testing.T) {
	expected := errors.New("generation failed")
	recorder := &taskRecorderFake{}

	_, err := GenerationTaskService{
		RequestIDs: taskRequestIDsFake{id: "request-1"},
		Recorders:  &taskRecorderFactoryFake{recorder: recorder},
		Generator:  &taskGenerationHandlerFake{err: expected},
	}.Generate(context.Background(), GenerationTaskCommand{
		Session: taskSession(),
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected generation error, got %v", err)
	}
	if got := recorder.events; len(got) != 3 || got[1] != "save" || got[2] != "close" {
		t.Fatalf("expected recorder to save and close on generation error, got %#v", got)
	}
}

func TestGenerationTaskServiceReportsRecorderErrors(t *testing.T) {
	saveErr := errors.New("save failed")
	closeErr := errors.New("close failed")
	var gotSaveErr error
	var gotCloseErr error

	_, err := GenerationTaskService{
		RequestIDs: taskRequestIDsFake{id: "request-1"},
		Recorders: &taskRecorderFactoryFake{recorder: &taskRecorderFake{
			saveErr:  saveErr,
			closeErr: closeErr,
		}},
		Generator: &taskGenerationHandlerFake{},
		OnSaveError: func(err error) {
			gotSaveErr = err
		},
		OnCloseError: func(err error) {
			gotCloseErr = err
		},
	}.Generate(context.Background(), GenerationTaskCommand{
		Session: taskSession(),
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !errors.Is(gotSaveErr, saveErr) {
		t.Fatalf("expected save error callback, got %v", gotSaveErr)
	}
	if !errors.Is(gotCloseErr, closeErr) {
		t.Fatalf("expected close error callback, got %v", gotCloseErr)
	}
}

type taskRequestIDsFake struct {
	id string
}

func (g taskRequestIDsFake) NewRequestID() string {
	return g.id
}

type taskRecorderFactoryFake struct {
	record   TaskRecord
	recorder GenerationTaskRecorder
}

func (f *taskRecorderFactoryFake) NewTaskRecorder(record TaskRecord) GenerationTaskRecorder {
	f.record = record
	return f.recorder
}

type taskRecorderAttachedKey struct{}

type taskRecorderFake struct {
	events   []string
	saveErr  error
	closeErr error
}

func (r *taskRecorderFake) Attach(ctx context.Context) context.Context {
	r.events = append(r.events, "attach")
	return context.WithValue(ctx, taskRecorderAttachedKey{}, true)
}

func (r *taskRecorderFake) Save(ctx context.Context) error {
	r.events = append(r.events, "save")
	return r.saveErr
}

func (r *taskRecorderFake) Close() error {
	r.events = append(r.events, "close")
	return r.closeErr
}

type taskGenerationHandlerFake struct {
	ctx     context.Context
	command AssistantGenerationCommand
	result  GenerationResponse
	err     error
}

func (h *taskGenerationHandlerFake) Generate(ctx context.Context, command AssistantGenerationCommand) (GenerationResponse, error) {
	h.ctx = ctx
	h.command = command
	if h.err != nil {
		return GenerationResponse{}, h.err
	}
	return h.result, nil
}

func taskSession() *session.Session {
	return &session.Session{
		ID:    "session-1",
		Model: "model-a",
	}
}
