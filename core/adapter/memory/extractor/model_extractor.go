package extractor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	domainagentrun "myai/core/domain/agentrun"
	domaingeneration "myai/core/domain/generation"
	domainmemory "myai/core/domain/memory"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
)

const (
	extractorVersion = "model-memory-v1"
	maxEventText     = 2000
	maxEvidenceText  = 14000
	maxCandidates    = 3
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|password|secret)\s*[:=]\s*[^\s,;]+`),
	regexp.MustCompile(`(?i)(bearer)\s+[a-z0-9._~+/=-]+`),
}

type ModelExtractor struct {
	Model        modelport.ChatModelPort
	DefaultScope domainmemory.Scope
}

func (extractor ModelExtractor) Version() string { return extractorVersion }

func (extractor ModelExtractor) Extract(ctx context.Context, run domainagentrun.Run, events []domainagentrun.Event) ([]domainmemory.CandidateDraft, error) {
	if extractor.Model == nil {
		return nil, errors.New("memory extraction model is nil")
	}
	if !run.IsTerminal() || run.Status == domainagentrun.StatusPaused || run.Status == domainagentrun.StatusCanceled {
		return nil, nil
	}
	evidence, err := json.Marshal(extractionEvidenceFromRun(run, events))
	if err != nil {
		return nil, fmt.Errorf("encode memory extraction evidence: %w", err)
	}
	settings := domaingeneration.SystemDefaults()
	settings.Temperature = 0.1
	settings.MaxOutputTokens = 1600
	generated, err := extractor.Model.Generate(ctx, modelport.GenerateRequest{
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleSystem, memoryExtractionSystemPrompt()),
			domainmessage.Text(domainmessage.RoleUser, "Extract reusable memory candidates from this AgentRun evidence:\n"+string(evidence)),
		},
		Settings: settings,
	})
	if err != nil {
		return nil, fmt.Errorf("generate memory candidates: %w", err)
	}
	output, err := decodeExtractionOutput(generated.Content)
	if err != nil {
		return nil, err
	}
	if len(output.Candidates) > maxCandidates {
		output.Candidates = output.Candidates[:maxCandidates]
	}
	defaultScope := extractor.DefaultScope
	defaultScope.Key = strings.TrimSpace(defaultScope.Key)
	if defaultScope.Validate() != nil {
		defaultScope = domainmemory.Scope{Type: domainmemory.ScopeGlobal}
	}
	drafts := make([]domainmemory.CandidateDraft, 0, len(output.Candidates))
	for _, item := range output.Candidates {
		kind := domainmemory.Kind(strings.TrimSpace(item.Kind))
		if kind == "" {
			if run.Status == domainagentrun.StatusFailed {
				kind = domainmemory.KindFailure
			} else {
				kind = domainmemory.KindExperience
			}
		}
		scope := extractedScopeOrDefault(item.ScopeType, item.ScopeKey, defaultScope)
		drafts = append(drafts, domainmemory.CandidateDraft{
			Title: strings.TrimSpace(item.Title), Kind: kind, Scope: scope,
			Tags: normalizeOutputTags(item.Tags), Confidence: item.Confidence,
			Content: domainmemory.Content{
				Goal: strings.TrimSpace(item.Goal), ApplicableContext: strings.TrimSpace(item.ApplicableContext),
				Approach: strings.TrimSpace(item.Approach), Result: strings.TrimSpace(item.Result),
				PainPoints: strings.TrimSpace(item.PainPoints), RootCause: strings.TrimSpace(item.RootCause),
				Lessons: strings.TrimSpace(item.Lessons), Verification: strings.TrimSpace(item.Verification),
			},
		})
	}
	return drafts, nil
}

func extractedScopeOrDefault(scopeType string, scopeKey string, fallback domainmemory.Scope) domainmemory.Scope {
	candidate := domainmemory.Scope{
		Type: domainmemory.ScopeType(strings.TrimSpace(scopeType)),
		Key:  strings.TrimSpace(scopeKey),
	}
	if candidate.Validate() == nil {
		return candidate
	}
	return fallback
}

type extractionEvidence struct {
	RunID        string          `json:"run_id"`
	SessionID    string          `json:"session_id"`
	Kind         string          `json:"kind"`
	Title        string          `json:"title"`
	Reason       string          `json:"reason"`
	Status       string          `json:"status"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Events       []evidenceEvent `json:"events"`
}

type evidenceEvent struct {
	Type         string `json:"type"`
	Title        string `json:"title,omitempty"`
	Content      string `json:"content,omitempty"`
	ToolName     string `json:"tool_name,omitempty"`
	Arguments    string `json:"arguments,omitempty"`
	Status       string `json:"status,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func extractionEvidenceFromRun(run domainagentrun.Run, events []domainagentrun.Event) extractionEvidence {
	result := extractionEvidence{
		RunID: run.ID, SessionID: run.SessionID, Kind: string(run.Kind), Title: sanitize(run.Title),
		Reason: sanitize(run.Reason), Status: string(run.Status), ErrorMessage: sanitize(run.ErrorMessage),
		Events: make([]evidenceEvent, 0, len(events)),
	}
	remaining := maxEvidenceText
	for _, event := range events {
		if remaining <= 0 {
			break
		}
		item := evidenceEvent{
			Type: string(event.Type), Title: sanitize(event.Title), ToolName: event.ToolName,
			Status: event.Status, ErrorCode: event.ErrorCode,
		}
		item.Content, remaining = boundedEvidence(event.Content, remaining)
		item.Arguments, remaining = boundedEvidence(event.Arguments, remaining)
		item.ErrorMessage, remaining = boundedEvidence(event.ErrorMessage, remaining)
		result.Events = append(result.Events, item)
	}
	return result
}

func boundedEvidence(value string, remaining int) (string, int) {
	value = sanitize(value)
	limit := maxEventText
	if remaining < limit {
		limit = remaining
	}
	value = truncateUTF8(value, limit)
	return value, remaining - len(value)
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	for _, pattern := range secretPatterns {
		value = pattern.ReplaceAllString(value, "$1=[REDACTED]")
	}
	return value
}

type extractionOutput struct {
	Candidates []extractionCandidate `json:"candidates"`
}

type extractionCandidate struct {
	Title             string   `json:"title"`
	Kind              string   `json:"kind"`
	ScopeType         string   `json:"scope_type"`
	ScopeKey          string   `json:"scope_key"`
	Tags              []string `json:"tags"`
	Goal              string   `json:"goal"`
	ApplicableContext string   `json:"applicable_context"`
	Approach          string   `json:"approach"`
	Result            string   `json:"result"`
	PainPoints        string   `json:"pain_points"`
	RootCause         string   `json:"root_cause"`
	Lessons           string   `json:"lessons"`
	Verification      string   `json:"verification"`
	Confidence        float64  `json:"confidence"`
}

func decodeExtractionOutput(content string) (extractionOutput, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return extractionOutput{}, errors.New("memory extractor returned no JSON object")
	}
	var output extractionOutput
	if err := json.Unmarshal([]byte(content[start:end+1]), &output); err != nil {
		return extractionOutput{}, fmt.Errorf("decode memory extractor output: %w", err)
	}
	return output, nil
}

func normalizeOutputTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func memoryExtractionSystemPrompt() string {
	return `You are a quality-controlled memory extractor for a coding agent.
Return exactly one JSON object with a "candidates" array and no markdown.
Create at most three candidates. Return an empty array for routine chat, canceled work, or evidence without reusable lessons.
Each candidate must contain: title, kind, scope_type, scope_key, tags, goal, applicable_context, approach, result, pain_points, root_cause, lessons, verification, confidence.
kind must be experience, failure, decision, or preference. confidence must be between 0 and 1.
scope_type must be global, workspace, project, or session. global requires an empty scope_key; other scopes require a scope_key.
Prefer concise Chinese text when the evidence is Chinese. Never include credentials, tokens, passwords, private keys, or unsupported conclusions.
Successful methods require verification evidence. Failed runs must be recorded as failure lessons, not recommended solutions.`
}
