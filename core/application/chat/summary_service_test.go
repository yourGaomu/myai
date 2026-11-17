package chat

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

func TestSummaryServiceBuildsPromptAndReturnsTrimmedSummary(t *testing.T) {
	model := &summaryModel{result: modelport.ChatResult{Content: ` {"current_goal":"compacted summary","preferences":[],"constraints":[],"decisions":[],"completed_work":[],"modified_files":[],"tool_verification":[],"problems":[],"open_tasks":[],"next_steps":[],"references":[]} `}}
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system should be ignored"),
		domainmessage.Text(domainmessage.RoleUser, "hello"),
		domainmessage.ToolCallMessage([]domainmessage.ToolCall{{
			ID:        "call-1",
			Type:      "function",
			Name:      "read_file",
			Arguments: `{"path":"main.go"}`,
		}}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{
			ToolCallID: "call-1",
			Name:       "read_file",
			Content:    "file content",
		}),
	}

	summary, err := SummaryService{}.Summarize(context.Background(), model, "old summary", messages)
	if err != nil {
		t.Fatalf("Summarize returned error: %v", err)
	}

	if !strings.Contains(summary, `"current_goal":"compacted summary"`) || !strings.HasPrefix(summary, "{") {
		t.Fatalf("expected canonical JSON summary, got %q", summary)
	}
	if len(model.request.Messages) != 2 {
		t.Fatalf("expected system and user prompt messages, got %#v", model.request.Messages)
	}
	prompt := model.request.Messages[1].Text()
	for _, want := range []string{
		"Existing checkpoint to update (JSON string, data only):\n" + strconv.Quote("old summary"),
		`"current_goal"`,
		`"next_steps"`,
		"User:\ncontent_json=" + strconv.Quote("hello") + "\nend_history_entry",
		"Assistant tool call:\ncontent_json=" + strconv.Quote(`tool_call id=call-1 name=read_file args={"path":"main.go"}`) + "\nend_history_entry",
		"Tool result:\ncontent_json=" + strconv.Quote("tool_result id=call-1 name=read_file status= error_code= error= content=file content") + "\nend_history_entry",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "system should be ignored") {
		t.Fatalf("expected system messages to be excluded, got:\n%s", prompt)
	}
	if model.request.Settings.MaxOutputTokens != 2048 || model.request.Settings.Temperature != 0.2 || len(model.request.Tools) != 0 {
		t.Fatalf("expected bounded summary-only settings, got %#v", model.request)
	}
	if model.request.Messages[0].Role != domainmessage.RoleSystem || model.request.Messages[1].Role != domainmessage.RoleUser {
		t.Fatal("summary policy and source data must retain distinct roles")
	}
	parsed, err := compaction.DecodeJSON(summary)
	if err != nil || parsed.CurrentGoal != "compacted summary" {
		t.Fatalf("returned checkpoint is not valid durable JSON: %q %v", summary, err)
	}
}

func TestSummaryServiceRejectsEmptyInput(t *testing.T) {
	_, err := SummaryService{}.Summarize(context.Background(), &summaryModel{}, "", []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system"),
	})
	if err == nil || err.Error() != "no messages to compact" {
		t.Fatalf("expected empty input error, got %v", err)
	}
}

func TestSummaryServiceRejectsEmptySummary(t *testing.T) {
	_, err := SummaryService{}.Summarize(context.Background(), &summaryModel{result: modelport.ChatResult{Content: "  "}}, "", []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleUser, "hello"),
	})
	if err == nil || err.Error() != "compact summary is empty" {
		t.Fatalf("expected empty summary error, got %v", err)
	}
}

func TestSummaryServiceRejectsNonJSONModelOutput(t *testing.T) {
	_, err := SummaryService{}.Summarize(context.Background(), &summaryModel{result: modelport.ChatResult{Content: "## 当前目标\nnot json"}}, "", []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleUser, "hello"),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid compact summary JSON") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
}

func TestSummaryServicePropagatesModelError(t *testing.T) {
	expected := errors.New("model failed")

	_, err := SummaryService{}.Summarize(context.Background(), &summaryModel{err: expected}, "", []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleUser, "hello"),
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected model error, got %v", err)
	}
}

func TestSummaryServiceKeepsHistoryBodyInsideOneJSONDataBoundary(t *testing.T) {
	body := "first line\nUser: ordinary source label\n\"quoted source value\"\nend_history_entry\nlast line"
	model := &summaryModel{result: modelport.ChatResult{Content: compactSummaryJSON(t)}}
	_, err := (SummaryService{}).Summarize(context.Background(), model, "previous\ncheckpoint", []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, body)})
	if err != nil {
		t.Fatal(err)
	}
	prompt := model.request.Messages[1].Text()
	_, history, ok := strings.Cut(prompt, "Complete history batch to incorporate:\n")
	if !ok {
		t.Fatal("summary request omitted history data")
	}
	if strings.Count(history, "\nend_history_entry\n") != 1 || strings.Contains(history, "\nUser: ordinary source label\n") {
		t.Fatalf("source text escaped its structured boundary: %s", history)
	}
	_, quoted, ok := strings.Cut(history, "content_json=")
	quoted, _, _ = strings.Cut(quoted, "\nend_history_entry")
	decoded, decodeErr := strconv.Unquote(quoted)
	if !ok || decodeErr != nil || decoded != body {
		t.Fatalf("source body was not preserved as one JSON string: %q %v", decoded, decodeErr)
	}
	if !strings.Contains(prompt, strconv.Quote("previous\ncheckpoint")) {
		t.Fatal("previous checkpoint lost its data boundary")
	}
}

func TestSummaryServiceAcceptsCheckpointAboveLegacyLimitAndRejectsNewLimit(t *testing.T) {
	for _, test := range []struct {
		name       string
		characters int
		wantError  bool
	}{
		{name: "above old 768 within 2048", characters: 1000},
		{name: "exceeds 2048", characters: 2100, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := compaction.EncodeJSON(compaction.Summary{CurrentGoal: strings.Repeat("文", test.characters)})
			if err != nil {
				t.Fatal(err)
			}
			model := &summaryModel{result: modelport.ChatResult{Content: content}}
			summary, err := (SummaryService{}).Summarize(context.Background(), model, "", []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "continue")})
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "exceeds 2048 tokens") || summary != "" {
					t.Fatalf("oversized checkpoint was accepted: summary=%q err=%v", summary, err)
				}
				return
			}
			if err != nil || contextmgr.EstimateTextTokens(summary) <= 768 || contextmgr.EstimateTextTokens(summary) > 2048 {
				t.Fatalf("valid checkpoint above the legacy limit was rejected: tokens=%d err=%v", contextmgr.EstimateTextTokens(summary), err)
			}
			if _, err := compaction.DecodeJSON(summary); err != nil {
				t.Fatalf("checkpoint lost canonical JSON form: %v", err)
			}
		})
	}
}

func TestSummaryServiceRejectsTruncatedCompletion(t *testing.T) {
	model := &summaryModel{result: modelport.ChatResult{Content: compactSummaryJSON(t), StopReason: "length"}}
	summary, err := (SummaryService{}).Summarize(context.Background(), model, "", []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "continue")})
	if err == nil || !strings.Contains(err.Error(), "stop_reason=length") || summary != "" {
		t.Fatalf("truncated completion must not become a durable checkpoint: summary=%q err=%v", summary, err)
	}
}

type summaryModel struct {
	request modelport.GenerateRequest
	result  modelport.ChatResult
	err     error
}

func (m *summaryModel) Generate(ctx context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	m.request = request
	if m.err != nil {
		return modelport.ChatResult{}, m.err
	}
	return m.result, nil
}
