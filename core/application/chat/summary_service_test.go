package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

func TestSummaryServiceBuildsPromptAndReturnsTrimmedSummary(t *testing.T) {
	model := &summaryModel{result: modelport.ChatResult{Content: "  compacted summary  "}}
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

	if summary != "compacted summary" {
		t.Fatalf("expected trimmed summary, got %q", summary)
	}
	if len(model.request.Messages) != 2 {
		t.Fatalf("expected system and user prompt messages, got %#v", model.request.Messages)
	}
	prompt := model.request.Messages[1].Text()
	for _, want := range []string{
		"Existing summary:\nold summary",
		"User:\nhello",
		"Assistant tool call:\ntool_call id=call-1 name=read_file",
		"Tool result:\ntool_result id=call-1 name=read_file content=file content",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "system should be ignored") {
		t.Fatalf("expected system messages to be excluded, got:\n%s", prompt)
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

func TestSummaryServicePropagatesModelError(t *testing.T) {
	expected := errors.New("model failed")

	_, err := SummaryService{}.Summarize(context.Background(), &summaryModel{err: expected}, "", []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleUser, "hello"),
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected model error, got %v", err)
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
