package service

import (
	"context"
	"errors"
	"testing"
	"time"

	subagentevents "myai/core/adapter/subagent/events"
	subagentmemory "myai/core/adapter/subagent/memory"
	subagentquery "myai/core/application/subagent/query"
	domainsubagent "myai/core/domain/subagent"
	domaintool "myai/core/domain/tool"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
	subagentport "myai/core/port/subagent"
)

type inspectionSessions struct {
	record repository.SessionRecord
	err    error
}

func (s inspectionSessions) GetSession(context.Context, string) (repository.SessionRecord, error) {
	return s.record, s.err
}

func TestInspectionValidatesBothSidesWithoutStartingOrSwitchingSession(t *testing.T) {
	ctx := context.Background()
	repo := subagentmemory.NewRepository()
	task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", ChildSessionID: "child", CurrentRunID: "run", Status: domainsubagent.TaskStatusRunning}
	if err := repo.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Tasks: repo, Runs: repo, EventHistory: repo,
		SessionRecords: inspectionSessions{record: repository.SessionRecord{ID: "child", Kind: "subagent", ParentSessionID: "parent", ParentTaskID: "task"}}}
	result, err := svc.InspectTask(ctx, subagentquery.InspectTask{ParentSessionID: "parent", TaskID: "task"})
	if err != nil || !result.SessionReady || result.Task.ChildSessionID != "child" {
		t.Fatalf("inspection = %#v, %v", result, err)
	}
	for _, parent := range []string{"", "other-parent"} {
		if _, err := svc.InspectTask(ctx, subagentquery.InspectTask{ParentSessionID: parent, TaskID: "task"}); err == nil {
			t.Fatal("accepted wrong parent")
		}
	}
	svc.SessionRecords = inspectionSessions{record: repository.SessionRecord{ID: "child", Kind: "subagent", ParentSessionID: "parent", ParentTaskID: "other-task"}}
	if _, err := svc.InspectTask(ctx, subagentquery.InspectTask{ParentSessionID: "parent", TaskID: "task"}); !errors.Is(err, subagentport.ErrNotFound) {
		t.Fatalf("accepted mismatched child: %v", err)
	}
	svc.SessionRecords = inspectionSessions{err: repository.ErrNotFound}
	result, err = svc.InspectTask(ctx, subagentquery.InspectTask{ParentSessionID: "parent", TaskID: "task"})
	if err != nil || result.SessionReady {
		t.Fatalf("uncreated child must remain inspectable: %#v %v", result, err)
	}
}

func TestTaskEventReplayPagesStayScopedAndFreezeWatermark(t *testing.T) {
	ctx := context.Background()
	repo := subagentmemory.NewRepository()
	task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", CurrentRunID: "run"}
	_ = repo.SaveTask(ctx, task)
	for i := uint64(1); i <= 6; i++ {
		event := subagentport.TaskEvent{Task: task, Sequence: i, RunID: "run", Content: "实时中文输出"}
		if i%2 == 0 {
			event.Task.ParentSessionID = "other-parent"
		}
		if err := repo.SaveTaskEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	svc := &Service{Tasks: repo, EventHistory: repo}
	query := subagentquery.TaskEvents{ParentSessionID: "parent", TaskID: "task", RunID: "run", Limit: 2}
	page, err := svc.ListTaskEvents(ctx, query)
	if err != nil || !page.HasMore || page.NextSequence != 3 || page.ThroughSequence != 6 || len(page.Events) != 2 {
		t.Fatalf("first page: %#v %v", page, err)
	}
	_ = repo.SaveTaskEvent(ctx, subagentport.TaskEvent{Task: task, Sequence: 7, RunID: "run"})
	query.AfterSequence, query.ThroughSequence = page.NextSequence, page.ThroughSequence
	page, err = svc.ListTaskEvents(ctx, query)
	if err != nil || page.HasMore || len(page.Events) != 1 || page.Events[0].Sequence != 5 || page.NextSequence != 6 {
		t.Fatalf("frozen page: %#v %v", page, err)
	}
	query.AfterSequence, query.ThroughSequence = page.NextSequence, 0
	page, err = svc.ListTaskEvents(ctx, query)
	if err != nil || len(page.Events) != 1 || page.Events[0].Sequence != 7 {
		t.Fatalf("catch up: %#v %v", page, err)
	}
	query.ParentSessionID = "other-parent"
	if _, err := svc.ListTaskEvents(ctx, query); !errors.Is(err, subagentport.ErrNotFound) {
		t.Fatalf("cross-parent replay allowed: %v", err)
	}
}

func TestTaskStreamPublishesIdentifiedToolsAssetsAndRejectsStaleRun(t *testing.T) {
	ctx := context.Background()
	repo := subagentmemory.NewRepository()
	task := domainsubagent.Task{ID: "task", ParentSessionID: "parent", ChildSessionID: "child", CurrentRunID: "run"}
	_ = repo.SaveTask(ctx, task)
	svc := &Service{Tasks: repo, Events: subagentevents.NewBus(repo)}
	stream := svc.taskStream(ctx, "task", "run")
	stream.OnAnswer("正在生成图片")
	stream.OnToolCallEvent(modelport.ToolCallEvent{ToolCallID: "call", Name: "share_file", Arguments: `{}`})
	stream.OnToolResult(modelport.ToolResultEvent{ToolCallID: "call", Name: "share_file", Output: domaintool.ToolOutput{Content: "完成"}})
	expires := time.Now().Add(time.Hour)
	stream.OnAsset(domaintool.SharedAsset{ID: "asset", SessionID: "child", ToolCallID: "call", ContentType: "image/png", ShortURL: "https://example.com/preview", ExpiresAt: &expires})
	expires = time.Time{}
	events, err := repo.ListTaskEvents(ctx, "parent", 0, 100)
	if err != nil || len(events) != 4 {
		t.Fatalf("events: %#v %v", events, err)
	}
	if events[1].ToolCallID != "call" || events[2].ToolCallID != "call" || events[3].Assets[0].ID != "asset" || events[3].Assets[0].ExpiresAt.IsZero() {
		t.Fatalf("event identity or snapshot lost: %#v", events)
	}
	task.CurrentRunID = "new-run"
	_ = repo.SaveTask(ctx, task)
	stream.OnAnswer("旧运行迟到的结果")
	latest, _ := repo.LatestTaskEventSequence(ctx)
	if latest != 4 {
		t.Fatalf("stale run published an event: %d", latest)
	}
}
