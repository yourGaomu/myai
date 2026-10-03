package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

const semanticAutoPlanPrompt = `You are the intent classifier for a coding assistant.
Classify the user's latest request using only the allowed JSON shape below.
The request may be in Chinese or English. Choose implementation only when the
user is asking the assistant to implement, modify, debug, configure, build, or
otherwise change code or a project. Questions asking how, why, whether, or if
something is possible are explanation unless they also explicitly ask the
assistant to perform the change. Writing, translation, brainstorming, and
general chat are not implementation tasks.

Return exactly one JSON object, with no Markdown:
{"intent":"implementation|resume_plan|explanation|conversation","confidence":0.0,"reason":"short reason"}`

// ModelAutoPlanClassifier uses a short, tool-free model turn to classify
// ambiguous requests semantically. It is intentionally separate from the
// normal generation service so classification cannot execute tools or mutate
// the workspace.
type ModelAutoPlanClassifier struct {
	Models        modelport.Registry
	Metadata      modelport.MetadataProvider
	Timeout       time.Duration
	MinConfidence float64
}

var _ AutoPlanClassifier = ModelAutoPlanClassifier{}

func (c ModelAutoPlanClassifier) Classify(ctx context.Context, current *session.Session, input string) (AutoPlanDecision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return AutoPlanDecision{Intent: AutoPlanIntentConversation, Action: AutoPlanActionChat, Reason: "empty request"}, nil
	}
	if current == nil {
		return AutoPlanDecision{}, errors.New("semantic auto-plan classification requires a session")
	}
	if current.Kind == session.KindSubagent {
		return AutoPlanDecision{Intent: AutoPlanIntentConversation, Action: AutoPlanActionChat, Reason: "subagent sessions do not recursively start autonomous planning"}, nil
	}
	// Avoid a second model request for clear-cut turns. Semantic classification
	// is reserved for short or ambiguous follow-ups that have project context.
	ruleDecision, _ := (RuleBasedAutoPlanClassifier{}).Classify(ctx, current, input)
	if ruleDecision.Action != AutoPlanActionChat || ruleDecision.Intent == AutoPlanIntentExplanation || !hasImplementationContext(current) {
		return ruleDecision, nil
	}
	if c.Models == nil {
		return AutoPlanDecision{}, errors.New("semantic auto-plan model registry is nil")
	}
	model := c.Models.GetModel(strings.TrimSpace(current.Model))
	if model == nil {
		return AutoPlanDecision{}, fmt.Errorf("semantic auto-plan model not found: %s", strings.TrimSpace(current.Model))
	}

	settings := generation.SystemDefaults()
	if c.Metadata != nil {
		if info, ok := c.Metadata.GetModelInfo(current.Model); ok {
			resolved, err := generation.Resolve(info.DefaultGenerationSettings, current.GenerationSettings)
			if err != nil {
				return AutoPlanDecision{}, err
			}
			settings = resolved
		}
	}

	request := modelport.GenerateRequest{
		Messages: classifierMessages(current, input),
		Settings: settings,
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	classificationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := model.Generate(classificationCtx, request)
	if err != nil {
		return AutoPlanDecision{}, fmt.Errorf("semantic auto-plan classification failed: %w", err)
	}
	return parseAutoPlanDecision(result.Content, c.minConfidence(), current)
}

func (c ModelAutoPlanClassifier) minConfidence() float64 {
	if c.MinConfidence > 0 {
		return c.MinConfidence
	}
	return 0.65
}

func classifierMessages(current *session.Session, input string) []domainmessage.Message {
	// Only recent user text is included. Tool output and assistant text are
	// deliberately excluded because neither is needed to classify the request.
	const maxContextMessages = 4
	const maxContextChars = 4000
	history := make([]string, 0, maxContextMessages)
	for index := len(current.Messages) - 1; index >= 0 && len(history) < maxContextMessages; index-- {
		message := current.Messages[index]
		if message.Role != domainmessage.RoleUser || message.IsSynthetic() {
			continue
		}
		text := strings.TrimSpace(message.Text())
		if text == "" {
			continue
		}
		if runes := []rune(text); len(runes) > maxContextChars {
			text = string(runes[:maxContextChars])
		}
		history = append(history, text)
	}
	var builder strings.Builder
	builder.WriteString("Latest request:\n")
	builder.WriteString(input)
	if len(history) > 0 {
		builder.WriteString("\n\nRecent user context (oldest to newest):\n")
		for index := len(history) - 1; index >= 0; index-- {
			builder.WriteString("- ")
			builder.WriteString(history[index])
			builder.WriteByte('\n')
		}
	}
	return []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, semanticAutoPlanPrompt),
		domainmessage.Text(domainmessage.RoleUser, builder.String()),
	}
}

func parseAutoPlanDecision(content string, minConfidence float64, current *session.Session) (AutoPlanDecision, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return AutoPlanDecision{}, errors.New("semantic auto-plan classifier returned no JSON object")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content[start:end+1]), &raw); err != nil {
		return AutoPlanDecision{}, fmt.Errorf("decode semantic auto-plan decision: %w", err)
	}
	var intentText string
	if err := decodeClassifierField(raw, "intent", &intentText); err != nil {
		return AutoPlanDecision{}, errors.New("semantic auto-plan classifier returned invalid intent")
	}
	intent := AutoPlanIntent(strings.ToLower(strings.TrimSpace(intentText)))
	switch intent {
	case AutoPlanIntentImplementation, AutoPlanIntentResumePlan, AutoPlanIntentExplanation, AutoPlanIntentConversation:
	default:
		return AutoPlanDecision{}, fmt.Errorf("semantic auto-plan classifier returned invalid intent %q", intentText)
	}
	var confidence float64
	if decodeClassifierField(raw, "confidence", &confidence) != nil || confidence < 0 || confidence > 1 {
		return AutoPlanDecision{}, errors.New("semantic auto-plan classifier returned incomplete confidence fields")
	}
	action := AutoPlanActionChat
	if intent == AutoPlanIntentImplementation && confidence >= minConfidence {
		action = AutoPlanActionPlanExecute
	} else if intent == AutoPlanIntentResumePlan && confidence >= minConfidence && hasExecutablePlan(current) {
		action = AutoPlanActionResumePlan
	}
	var reason string
	_ = decodeClassifierField(raw, "reason", &reason)
	return AutoPlanDecision{
		Intent: intent, Action: action,
		Confidence: confidence,
		Reason:     strings.TrimSpace(reason),
	}, nil
}

func decodeClassifierField[T any](payload map[string]json.RawMessage, key string, target *T) error {
	value, ok := payload[key]
	if !ok {
		return errors.New("classifier field is missing")
	}
	return json.Unmarshal(value, target)
}
