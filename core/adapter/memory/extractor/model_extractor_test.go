package extractor

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	domainagentrun "myai/core/domain/agentrun"
	domainmemory "myai/core/domain/memory"
	modelport "myai/core/port/model"
)

func TestModelExtractorRedactsEvidenceAndFallsBackFromInvalidScope(t *testing.T) {
	model := &recordingModel{result: modelport.ChatResult{Content: `{
  "candidates": [{
    "title": "Durable extraction jobs",
    "kind": "experience",
    "scope_type": "tenant",
    "scope_key": "ignored",
    "tags": ["Go", "go", "Recovery"],
    "goal": "Recover interrupted extraction",
    "approach": "Persist before scheduling",
    "confidence": 0.9
  }]
}`}}
	extractor := ModelExtractor{
		Model:        model,
		DefaultScope: domainmemory.Scope{Type: domainmemory.ScopeWorkspace, Key: `D:\Go_All\myai`},
	}
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	finishedAt := now.Add(time.Minute)
	run := domainagentrun.Run{
		ID: "run-1", SessionID: "session-1", Kind: domainagentrun.KindChat,
		Status: domainagentrun.StatusSucceeded, Reason: "api_key=super-secret", StartedAt: now, FinishedAt: &finishedAt,
	}
	events := []domainagentrun.Event{{
		ID: "event-1", RunID: run.ID, SessionID: run.SessionID, Sequence: 1,
		Type: domainagentrun.EventTypeToolResult, Content: "Authorization: Bearer abc.def.ghi", CreatedAt: now,
	}}

	drafts, err := extractor.Extract(context.Background(), run, events)
	if err != nil {
		t.Fatalf("extract memories: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("expected one draft, got %#v", drafts)
	}
	if drafts[0].Scope != extractor.DefaultScope {
		t.Fatalf("invalid model scope did not fall back: %#v", drafts[0].Scope)
	}
	if len(drafts[0].Tags) != 2 || drafts[0].Tags[0] != "go" || drafts[0].Tags[1] != "recovery" {
		t.Fatalf("tags were not normalized: %#v", drafts[0].Tags)
	}
	if len(model.requests) != 1 || len(model.requests[0].Messages) != 2 {
		t.Fatalf("unexpected model requests: %#v", model.requests)
	}
	evidence := model.requests[0].Messages[1].Text()
	if strings.Contains(evidence, "super-secret") || strings.Contains(evidence, "abc.def.ghi") {
		t.Fatalf("secret leaked into extraction evidence: %s", evidence)
	}
	if !strings.Contains(evidence, "[REDACTED]") {
		t.Fatalf("redaction marker missing from evidence: %s", evidence)
	}
	if model.requests[0].Settings.Temperature != 0.1 || model.requests[0].Settings.MaxOutputTokens != 1600 {
		t.Fatalf("unexpected extraction settings: %#v", model.requests[0].Settings)
	}
}

func TestModelExtractorSkipsNonReusableAndNonTerminalRuns(t *testing.T) {
	model := &recordingModel{result: modelport.ChatResult{Content: `{"candidates":[]}`}}
	extractor := ModelExtractor{Model: model}
	run := domainagentrun.Run{ID: "run-1", SessionID: "session-1", Status: domainagentrun.StatusRunning, StartedAt: time.Now()}

	drafts, err := extractor.Extract(context.Background(), run, nil)
	if err != nil || len(drafts) != 0 {
		t.Fatalf("non-terminal run should be skipped: drafts=%#v err=%v", drafts, err)
	}
	if len(model.requests) != 0 {
		t.Fatalf("model was called for non-terminal run")
	}
}

func TestTruncateUTF8DoesNotSplitCharacters(t *testing.T) {
	value := strings.Repeat("你", 1000)
	truncated := truncateUTF8(value, 2000)
	if len(truncated) > 2000 || !utf8.ValidString(truncated) {
		t.Fatalf("invalid UTF-8 truncation: bytes=%d valid=%v", len(truncated), utf8.ValidString(truncated))
	}
}

type recordingModel struct {
	requests []modelport.GenerateRequest
	result   modelport.ChatResult
	err      error
}

func (model *recordingModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	model.requests = append(model.requests, request)
	return model.result, model.err
}
