package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	domaininteraction "myai/core/domain/interaction"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type AskUserTool struct{}

var _ tooldef.Tool = (*AskUserTool)(nil)
var _ tooldef.TurnBarrier = (*AskUserTool)(nil)
var _ tooldef.UserInteraction = (*AskUserTool)(nil)

func NewAskUserTool() *AskUserTool {
	return &AskUserTool{}
}

func (tool *AskUserTool) Name() string { return "ask_user" }

func (tool *AskUserTool) Description() string {
	return "Ask the user a focused clarification when a consequential ambiguity would otherwise risk implementing the wrong thing. The current turn pauses until the user answers. Do not ask questions that can be safely resolved from context."
}

func (tool *AskUserTool) Schema() any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"title": map[string]any{"type": "string", "description": "Short reason shown above the questions."},
			"questions": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 5,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"id":          map[string]any{"type": "string", "description": "Stable identifier used to associate this answer."},
						"prompt":      map[string]any{"type": "string", "description": "One concise question."},
						"options":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 8},
						"allow_other": map[string]any{"type": "boolean", "description": "Allow the user to provide a different free-text answer."},
					},
					"required": []string{"prompt"},
				},
			},
		},
		"required": []string{"questions"},
	}
}

func (tool *AskUserTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (tool *AskUserTool) IsTurnBarrier() bool            { return true }
func (tool *AskUserTool) RequiresUserInteraction() bool  { return true }

func (tool *AskUserTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	var input struct {
		Title     string `json:"title"`
		Questions []struct {
			ID         string   `json:"id"`
			Prompt     string   `json:"prompt"`
			Options    []string `json:"options"`
			AllowOther bool     `json:"allow_other"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tooldef.ToolOutput{}, fmt.Errorf("decode user questions: %w", err)
	}
	questions := make([]domaininteraction.Question, 0, len(input.Questions))
	for index, rawQuestion := range input.Questions {
		question := domaininteraction.Question{
			ID: strings.TrimSpace(rawQuestion.ID), Prompt: strings.TrimSpace(rawQuestion.Prompt),
			Options: append([]string(nil), rawQuestion.Options...), AllowOther: rawQuestion.AllowOther,
		}
		if question.ID == "" {
			question.ID = fmt.Sprintf("question_%d", index+1)
		}
		if len(question.Options) == 0 {
			question.AllowOther = true
		}
		for optionIndex := range question.Options {
			question.Options[optionIndex] = strings.TrimSpace(question.Options[optionIndex])
		}
		questions = append(questions, question)
	}
	execution, _ := toolruntime.CurrentExecution(ctx)
	request := domaininteraction.Request{
		ID: uuid.NewString(), SessionID: execution.SessionID, SourceSessionID: execution.SessionID,
		SourceTaskID: execution.TaskID, RequestID: execution.RequestID, RunID: execution.RunID,
		ToolCallID: execution.ToolCallID, Title: strings.TrimSpace(input.Title), Questions: questions,
	}
	if execution.ParentSessionID != "" {
		request.SessionID = execution.ParentSessionID
	}
	if err := domaininteraction.ValidateRequest(request); err != nil {
		return tooldef.ToolOutput{}, err
	}
	handler := toolruntime.UserQuestionHandlerFrom(ctx)
	if handler == nil {
		return tooldef.ToolOutput{}, errors.New("user interaction is unavailable for this turn")
	}
	answer, err := handler(ctx, request)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if answer.Cancelled {
		return tooldef.ToolOutput{}, errors.New("the user chose not to answer")
	}
	if err := domaininteraction.ValidateAnswer(request, answer); err != nil {
		return tooldef.ToolOutput{}, err
	}
	encoded, err := json.Marshal(map[string]any{"answers": answer.Answers})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return tooldef.SuccessOutput(string(encoded)), nil
}
