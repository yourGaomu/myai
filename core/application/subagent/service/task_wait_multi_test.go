package service

import (
	"context"
	"testing"
	"time"

	subagentevents "myai/core/adapter/subagent/events"
	"myai/core/adapter/subagent/memory"
	subagentcommand "myai/core/application/subagent/command"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
)

func TestWaitReturnsWhenAnyTargetReachesTerminalState(t *testing.T) {
	repository := memory.NewRepository()
	bus := subagentevents.NewBus()
	first := domainsubagent.Task{ID: "first", ParentSessionID: "parent", Status: domainsubagent.TaskStatusRunning}
	second := domainsubagent.Task{ID: "second", ParentSessionID: "parent", Status: domainsubagent.TaskStatusRunning}
	for _, task := range []domainsubagent.Task{first, second} {
		if err := repository.SaveTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}

	service := &Service{Tasks: repository, Events: bus}
	resultCh := make(chan struct {
		result subagentresult.Wait
		err    error
	}, 1)
	go func() {
		result, err := service.Wait(context.Background(), subagentcommand.WaitTask{
			Targets: []string{first.ID, second.ID}, ParentSessionID: "parent", Timeout: time.Second,
		})
		resultCh <- struct {
			result subagentresult.Wait
			err    error
		}{result: result, err: err}
	}()

	if err := second.MarkSucceeded("second done", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveTask(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	bus.TaskUpdated(context.Background(), second)

	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		result := outcome.result
		if result.Task.ID != second.ID || len(result.Tasks) != 1 || result.TimedOut {
			t.Fatalf("unexpected multi-target wait result: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not wake for the first completed target")
	}
}

func TestWaitReturnsAllInitiallyTerminalTargets(t *testing.T) {
	repository := memory.NewRepository()
	first := domainsubagent.Task{ID: "first", ParentSessionID: "parent", Status: domainsubagent.TaskStatusSucceeded}
	second := domainsubagent.Task{ID: "second", ParentSessionID: "parent", Status: domainsubagent.TaskStatusFailed}
	for _, task := range []domainsubagent.Task{first, second} {
		if err := repository.SaveTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}

	service := &Service{Tasks: repository}
	result, err := service.Wait(context.Background(), subagentcommand.WaitTask{
		Targets: []string{first.ID, second.ID}, ParentSessionID: "parent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.ID != first.ID || len(result.Tasks) != 2 || result.TimedOut {
		t.Fatalf("unexpected initial terminal result: %#v", result)
	}
}

func TestWaitTimeoutReturnsAllTargetSnapshots(t *testing.T) {
	repository := memory.NewRepository()
	for _, task := range []domainsubagent.Task{
		{ID: "first", ParentSessionID: "parent", Status: domainsubagent.TaskStatusRunning},
		{ID: "second", ParentSessionID: "parent", Status: domainsubagent.TaskStatusRunning},
	} {
		if err := repository.SaveTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}

	service := &Service{Tasks: repository, Events: subagentevents.NewBus()}
	result, err := service.Wait(context.Background(), subagentcommand.WaitTask{
		Targets: []string{"first", "second"}, ParentSessionID: "parent", Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut || len(result.Tasks) != 2 || result.Task.ID != "first" {
		t.Fatalf("unexpected timeout result: %#v", result)
	}
}

func TestWaitRejectsTargetOutsideParentSession(t *testing.T) {
	repository := memory.NewRepository()
	if err := repository.SaveTask(context.Background(), domainsubagent.Task{ID: "child", ParentSessionID: "other", Status: domainsubagent.TaskStatusRunning}); err != nil {
		t.Fatal(err)
	}
	service := &Service{Tasks: repository}
	if _, err := service.Wait(context.Background(), subagentcommand.WaitTask{Targets: []string{"child"}, ParentSessionID: "parent"}); err == nil {
		t.Fatal("expected parent session validation error")
	}
}
