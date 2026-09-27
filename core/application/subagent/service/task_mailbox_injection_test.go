package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"myai/core/adapter/subagent/memory"
	generationcommand "myai/core/application/chat/generation/command"
	subagentcommand "myai/core/application/subagent/command"
	domainsubagent "myai/core/domain/subagent"
	domainworkspace "myai/core/domain/workspace"
	subagentport "myai/core/port/subagent"
	"myai/core/session"
)

func TestInjectedMailboxIsAcknowledgedOnlyAfterModelSuccess(t *testing.T) {
	for _, test := range []struct {
		name      string
		fail      bool
		panicRun  bool
		remaining int
	}{
		{name: "success", remaining: 0},
		{name: "model error", fail: true, remaining: 1},
		{name: "model panic", panicRun: true, remaining: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := memory.NewRepository()
			task := domainsubagent.Task{
				ID: "task", ParentSessionID: "parent", ChildSessionID: "child", CurrentRunID: "run",
				Status:     domainsubagent.TaskStatusQueued,
				Definition: domainsubagent.DefinitionSnapshot{TimeoutSeconds: 5},
				Workspace:  domainworkspace.Reference{Mode: domainworkspace.IsolationModeDirect},
				Mailbox:    []domainsubagent.Message{{ID: "message", Content: "extra requirement", Status: domainsubagent.MessageStatusPending}},
			}
			run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusQueued, CreatedAt: time.Now().UTC()}
			if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
				t.Fatal(err)
			}
			service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository,
				Sessions: &fakeChildSessions{}, Runner: injectionRunner{fail: test.fail, panicRun: test.panicRun}}
			if !service.registerActiveRun(task.ID, run.ID) {
				t.Fatal("failed to register run")
			}
			service.execute(context.Background(), task.ID, run.ID, "model")
			stored, err := repository.GetTask(context.Background(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(stored.Mailbox) != test.remaining {
				t.Fatalf("unexpected mailbox after model: %#v", stored.Mailbox)
			}
			if test.remaining > 0 && stored.Mailbox[0].Status != domainsubagent.MessageStatusPending {
				t.Fatalf("failed model did not release injected input: %#v", stored.Mailbox[0])
			}
		})
	}
}

func TestSendMessagePersistsEnvelopeAndMailboxAtomically(t *testing.T) {
	for _, test := range []struct {
		name   string
		failed bool
		status domainsubagent.AgentMessageDeliveryStatus
	}{
		{name: "success", status: domainsubagent.AgentMessageDelivered},
		{name: "model error", failed: true, status: domainsubagent.AgentMessagePending},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := memory.NewRepository()
			task := domainsubagent.Task{
				ID: "task", ParentSessionID: "parent", ChildSessionID: "child", CurrentRunID: "run",
				Status:     domainsubagent.TaskStatusQueued,
				Definition: domainsubagent.DefinitionSnapshot{TimeoutSeconds: 5},
				Workspace:  domainworkspace.Reference{Mode: domainworkspace.IsolationModeDirect},
			}
			run := domainsubagent.Run{ID: "run", TaskID: task.ID, Status: domainsubagent.RunStatusQueued}
			if err := repository.SaveTaskAndRun(context.Background(), task, run); err != nil {
				t.Fatal(err)
			}
			service := &Service{Tasks: repository, Runs: repository, TaskRuns: repository, AgentMessages: repository,
				IDs: &sequenceIDs{}, Sessions: &fakeChildSessions{}, Runner: injectionRunner{fail: test.failed, onInjected: func() error {
					message, err := repository.GetAgentMessage(context.Background(), "message-1")
					if err != nil {
						return err
					}
					if message.Status != domainsubagent.AgentMessageDelivering || message.DeliveryAttempts != 1 {
						return errors.New("agent message was not marked delivering during model run")
					}
					return nil
				}}}
			command := subagentcommand.SendMessage{TaskID: task.ID, ParentSessionID: task.ParentSessionID,
				MessageID: "message-1", Content: "extra requirement"}
			if _, err := service.SendMessage(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			if _, err := service.SendMessage(context.Background(), command); err != nil {
				t.Fatalf("idempotent retry failed: %v", err)
			}
			command.Content = "different requirement"
			if _, err := service.SendMessage(context.Background(), command); err == nil {
				t.Fatal("same message ID accepted different content")
			}
			command.Content = "extra requirement"
			command.Kind = domainsubagent.AgentMessageKindUserInput
			if _, err := service.SendMessage(context.Background(), command); err == nil {
				t.Fatal("same message ID accepted different sender semantics")
			}
			stored, err := repository.GetTask(context.Background(), task.ID)
			if err != nil || len(stored.Mailbox) != 1 {
				t.Fatalf("retry duplicated mailbox input: %#v, %v", stored.Mailbox, err)
			}
			message, err := repository.GetAgentMessage(context.Background(), command.MessageID)
			if err != nil || message.RecipientAgentID != task.ChildSessionID || message.Kind != domainsubagent.AgentMessageKindInterAgent {
				t.Fatalf("durable envelope is incorrect: %#v, %v", message, err)
			}
			if !service.registerActiveRun(task.ID, run.ID) {
				t.Fatal("failed to register run")
			}
			service.execute(context.Background(), task.ID, run.ID, "model")
			message, err = repository.GetAgentMessage(context.Background(), command.MessageID)
			if err != nil || message.Status != test.status {
				t.Fatalf("unexpected delivery state: %#v, %v", message, err)
			}
			stored, err = repository.GetTask(context.Background(), task.ID)
			if err != nil || len(stored.Mailbox) != map[bool]int{false: 0, true: 1}[test.failed] {
				t.Fatalf("mailbox and envelope diverged: %#v, %v", stored.Mailbox, err)
			}
		})
	}
}

type injectionRunner struct {
	fail       bool
	panicRun   bool
	onInjected func() error
}

func (runner injectionRunner) Run(ctx context.Context, request subagentport.AgentRunRequest) (subagentport.AgentRunResult, error) {
	hook := generationcommand.AfterToolRoundFrom(ctx)
	if hook == nil {
		return subagentport.AgentRunResult{}, errors.New("after-tool hook is missing")
	}
	current := &session.Session{ID: request.SessionID}
	if err := hook(ctx, current); err != nil {
		return subagentport.AgentRunResult{}, err
	}
	if len(current.Messages) != 1 || current.Messages[0].Text() != "extra requirement" {
		return subagentport.AgentRunResult{}, errors.New("mailbox input was not injected")
	}
	if runner.onInjected != nil {
		if err := runner.onInjected(); err != nil {
			return subagentport.AgentRunResult{}, err
		}
	}
	if runner.panicRun {
		panic("model panic")
	}
	if runner.fail {
		return subagentport.AgentRunResult{}, errors.New("model failed")
	}
	return subagentport.AgentRunResult{Content: "handled extra requirement"}, nil
}
