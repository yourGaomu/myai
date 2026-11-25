package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	domaininteraction "myai/core/domain/interaction"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type AskUserTool struct{}

var _ tooldef.Tool = (*AskUserTool)(nil)
var _ tooldef.TurnBarrier = (*AskUserTool)(nil)
var _ tooldef.UserInteraction = (*AskUserTool)(nil)

func NewAskUserTool() *AskUserTool     { return &AskUserTool{} }
func (tool *AskUserTool) Name() string { return "ask_user" }

func (tool *AskUserTool) Description() string {
	return "Ask one short round of focused questions only when a high-impact decision remains unresolved after checking the conversation and relevant workspace files, and guessing would cause meaningful rework. Broad requests alone are not a reason to ask. Include decision_impact=high, a concise decision_reason, and context_evidence. Use read-only workspace tools before claiming repository evidence. Do not ask about facts that can be inspected or decisions the user delegated to you. Provide concrete choices, a grounded recommendation where possible, and allow custom replies. If the user skips, continue only with a safe default; otherwise leave the blocked part undone and explain why."
}

func (tool *AskUserTool) Schema() any {
	choice := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"label":       map[string]any{"type": "string", "minLength": 1, "description": "Concise choice label."},
			"description": map[string]any{"type": "string", "description": "Short grounded rationale, trade-off, or best-for note."},
			"recommended": map[string]any{"type": "boolean", "description": "Whether this is the grounded recommended choice."},
		},
		"required": []string{"label"},
	}
	evidence := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"source":  map[string]any{"type": "string", "enum": []string{"conversation", "workspace"}},
			"path":    map[string]any{"type": "string", "description": "Required for workspace evidence; relative path actually checked with a read-only workspace tool."},
			"tool":    map[string]any{"type": "string", "enum": []string{"list_files", "read_file", "search_files"}, "description": "Read-only workspace tool that successfully inspected this path."},
			"finding": map[string]any{"type": "string", "minLength": 1, "description": "Short factual finding, not an instruction."},
		},
		"required": []string{"source", "finding"},
	}
	question := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"id":           map[string]any{"type": "string", "minLength": 1},
			"prompt":       map[string]any{"type": "string", "minLength": 1, "description": "One concise, actionable question."},
			"multi_select": map[string]any{"type": "boolean"},
			"allow_other":  map[string]any{"type": "boolean", "description": "Defaults to true when omitted."},
			"choices":      map[string]any{"type": "array", "maxItems": 8, "items": choice},
		},
		"required": []string{"id", "prompt", "multi_select", "choices"},
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"decision_impact":         map[string]any{"type": "string", "enum": []string{"high"}},
			"decision_reason":         map[string]any{"type": "string", "minLength": 1, "description": "Why the unresolved decision materially affects the result and cannot be inferred from checked context."},
			"context_evidence":        map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": evidence},
			"safe_default_if_skipped": map[string]any{"type": "string", "description": "Optional safe fallback; omit if there is none."},
			"title":                   map[string]any{"type": "string", "minLength": 1, "description": "Short card header."},
			"questions":               map[string]any{"type": "array", "minItems": 1, "maxItems": 5, "items": question},
		},
		"required": []string{"decision_impact", "decision_reason", "context_evidence", "title", "questions"},
	}
}

func (tool *AskUserTool) Permission() tooldef.Permission { return tooldef.PermissionRead }
func (tool *AskUserTool) IsTurnBarrier() bool            { return true }
func (tool *AskUserTool) RequiresUserInteraction() bool  { return true }

type askUserEvidence struct {
	Source  string `json:"source"`
	Path    string `json:"path,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Finding string `json:"finding"`
}

type askUserQuestionInput struct {
	ID          string                     `json:"id"`
	Prompt      string                     `json:"prompt"`
	MultiSelect *bool                      `json:"multi_select"`
	AllowOther  *bool                      `json:"allow_other,omitempty"`
	Choices     []domaininteraction.Option `json:"choices"`
}

type askUserInput struct {
	DecisionImpact       string                 `json:"decision_impact"`
	DecisionReason       string                 `json:"decision_reason"`
	ContextEvidence      []askUserEvidence      `json:"context_evidence"`
	SafeDefaultIfSkipped string                 `json:"safe_default_if_skipped,omitempty"`
	Title                string                 `json:"title"`
	Questions            []askUserQuestionInput `json:"questions"`
}

type askUserAnswerOutput struct {
	Selected []string `json:"selected,omitempty"`
	Other    string   `json:"other,omitempty"`
}

type askUserToolOutput struct {
	StructuredAnswers    map[string]askUserAnswerOutput `json:"structured_answers,omitempty"`
	AdoptedDefaults      bool                           `json:"adopted_defaults,omitempty"`
	Skipped              bool                           `json:"skipped,omitempty"`
	SafeDefaultIfSkipped string                         `json:"safe_default_if_skipped,omitempty"`
}

func (tool *AskUserTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.DisallowUnknownFields()
	var input askUserInput
	if err := decoder.Decode(&input); err != nil {
		return tooldef.ToolOutput{}, fmt.Errorf("decode user questions: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return tooldef.ToolOutput{}, errors.New("decode user questions: trailing JSON value")
	}
	if input.DecisionImpact != "high" || strings.TrimSpace(input.DecisionReason) == "" {
		return tooldef.ToolOutput{}, errors.New("ask_user requires a high-impact decision and a concise reason")
	}
	if strings.TrimSpace(input.Title) == "" {
		return tooldef.ToolOutput{}, errors.New("ask_user title is required")
	}
	if len(input.ContextEvidence) == 0 || len(input.ContextEvidence) > 8 {
		return tooldef.ToolOutput{}, errors.New("ask_user requires between one and eight checked context evidence entries")
	}
	if err := validateAskUserEvidence(ctx, input.ContextEvidence); err != nil {
		return tooldef.ToolOutput{}, err
	}
	questions := make([]domaininteraction.Question, 0, len(input.Questions))
	for index, raw := range input.Questions {
		if strings.TrimSpace(raw.ID) == "" {
			return tooldef.ToolOutput{}, fmt.Errorf("question %d id is required", index+1)
		}
		if raw.MultiSelect == nil {
			return tooldef.ToolOutput{}, fmt.Errorf("question %q must specify multi_select", raw.ID)
		}
		if raw.Choices == nil {
			return tooldef.ToolOutput{}, fmt.Errorf("question %q must specify choices as an array", raw.ID)
		}
		allowOther := true
		if raw.AllowOther != nil {
			allowOther = *raw.AllowOther
		}
		question := domaininteraction.Question{
			ID: strings.TrimSpace(raw.ID), Prompt: strings.TrimSpace(raw.Prompt),
			MultiSelect: *raw.MultiSelect, AllowOther: allowOther,
			Choices: append([]domaininteraction.Option(nil), raw.Choices...),
		}
		recommendedSeen := false
		for choiceIndex := range question.Choices {
			choice := &question.Choices[choiceIndex]
			choice.Label = strings.TrimSpace(choice.Label)
			choice.Description = truncateRunes(strings.TrimSpace(choice.Description), 120)
			if !question.MultiSelect && choice.Recommended {
				if recommendedSeen {
					slog.Warn("ask_user single-select question has multiple recommendations; later flag removed", "question_id", question.ID)
					choice.Recommended = false
				} else {
					recommendedSeen = true
				}
			}
		}
		questions = append(questions, question)
	}
	requestID, err := uuid.NewRandom()
	if err != nil {
		return tooldef.ToolOutput{}, fmt.Errorf("create user question id: %w", err)
	}
	execution, _ := toolruntime.CurrentExecution(ctx)
	request := domaininteraction.Request{
		ID: requestID.String(), SessionID: execution.SessionID, SourceSessionID: execution.SessionID,
		SourceTaskID: execution.TaskID, RequestID: execution.RequestID, RunID: execution.RunID,
		ToolCallID: execution.ToolCallID, Title: strings.TrimSpace(input.Title), Questions: questions,
	}
	if execution.ParentSessionID != "" {
		request.SessionID = execution.ParentSessionID
	}
	if err := domaininteraction.ValidateRequest(request); err != nil {
		return tooldef.ToolOutput{}, err
	}
	if execution.IsSubagent {
		return tooldef.ToolOutput{}, errors.New("subagents cannot ask the user directly; report the unresolved decision to the parent agent")
	}
	handler := toolruntime.UserQuestionHandlerFrom(ctx)
	if handler == nil {
		return tooldef.ToolOutput{}, errors.New("user interaction is unavailable for this turn")
	}
	answered := false
	if budget := toolruntime.AskUserBudgetFrom(ctx); budget != nil {
		if err := budget.Reserve(input.DecisionReason); err != nil {
			return tooldef.ToolOutput{}, err
		}
		defer func() { budget.Finish(answered) }()
	}
	answer, err := handler(ctx, request)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if answer.Cancelled {
		return marshalAskUserOutput(askUserToolOutput{Skipped: true, SafeDefaultIfSkipped: strings.TrimSpace(input.SafeDefaultIfSkipped)})
	}
	if err := domaininteraction.ValidateAnswer(request, answer); err != nil {
		return tooldef.ToolOutput{}, err
	}
	answered = true
	answer = domaininteraction.NormalizeAnswer(request, answer)
	return marshalAskUserOutput(askUserToolOutput{StructuredAnswers: structuredAnswerOutput(answer.Structured), AdoptedDefaults: answer.AdoptedDefaults})
}

func validateAskUserEvidence(ctx context.Context, evidence []askUserEvidence) error {
	execution, _ := toolruntime.CurrentExecution(ctx)
	tracker := toolruntime.WorkspaceInspectionTrackerFrom(ctx)
	seen := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		finding := strings.TrimSpace(item.Finding)
		if finding == "" {
			return errors.New("context evidence finding is required")
		}
		switch item.Source {
		case "conversation":
			if strings.TrimSpace(item.Path) != "" || strings.TrimSpace(item.Tool) != "" {
				return errors.New("conversation evidence must not include workspace path or tool")
			}
		case "workspace":
			path, toolName := strings.TrimSpace(item.Path), strings.TrimSpace(item.Tool)
			if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" || (toolName != "list_files" && toolName != "read_file" && toolName != "search_files") || tracker == nil || !tracker.HasInspection(execution.WorkspaceRoot, path, toolName) {
				return errors.New("workspace evidence was not successfully inspected with the stated tool in this turn; cite a relative path checked with a read-only workspace tool before asking")
			}
		default:
			return fmt.Errorf("unsupported context evidence source %q", item.Source)
		}
		key := item.Source + "\x00" + strings.TrimSpace(item.Tool) + "\x00" + strings.TrimSpace(item.Path) + "\x00" + finding
		if _, ok := seen[key]; ok {
			return errors.New("duplicate context evidence")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func truncateRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max-1]) + "…"
}

func structuredAnswerOutput(values map[string]domaininteraction.QuestionAnswer) map[string]askUserAnswerOutput {
	result := make(map[string]askUserAnswerOutput, len(values))
	for id, answer := range values {
		result[id] = askUserAnswerOutput{Selected: append([]string(nil), answer.Selected...), Other: answer.Other}
	}
	return result
}

func marshalAskUserOutput(output askUserToolOutput) (tooldef.ToolOutput, error) {
	encoded, err := json.Marshal(output)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return tooldef.SuccessOutput(string(encoded)), nil
}
