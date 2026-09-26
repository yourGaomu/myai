package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	domainmessage "myai/core/domain/message"
	intentport "myai/core/port/intent"
	"myai/core/session"
)

const intentTraceRetention = 7 * 24 * time.Hour

type IntentController struct {
	mu      sync.RWMutex
	config  intentport.Config
	store   intentport.Store
	client  intentport.Client
	builtin AutoPlanClassifier
}

func NewIntentController(ctx context.Context, store intentport.Store, client intentport.Client, builtin AutoPlanClassifier) (*IntentController, error) {
	if store == nil || client == nil || builtin == nil {
		return nil, errors.New("intent controller dependencies are nil")
	}
	config, err := store.LoadConfig(ctx)
	if errors.Is(err, intentport.ErrNotFound) {
		config = intentport.DefaultConfig()
	} else if err != nil {
		return nil, err
	}
	if err := validateIntentConfig(config); err != nil {
		return nil, fmt.Errorf("stored intent configuration is invalid: %w", err)
	}
	return &IntentController{config: config, store: store, client: client, builtin: builtin}, nil
}

func (c *IntentController) Config() intentport.ConfigView {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.View()
}

func (c *IntentController) SaveConfig(ctx context.Context, next intentport.Config, clearAPIKey bool) (intentport.ConfigView, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if next.BaseURL == "" {
		next.BaseURL = c.config.BaseURL
	}
	if next.Model == "" {
		next.Model = c.config.Model
	}
	if next.PlanConfidence == 0 {
		next.PlanConfidence = c.config.PlanConfidence
	}
	if next.ExecuteConfidence == 0 {
		next.ExecuteConfidence = c.config.ExecuteConfidence
	}
	if next.APIKey == "" && !clearAPIKey {
		next.APIKey = c.config.APIKey
	}
	next.APIKey = strings.TrimSpace(next.APIKey)
	next.BaseURL = strings.TrimRight(strings.TrimSpace(next.BaseURL), "/")
	next.Model = strings.TrimSpace(next.Model)
	if err := validateIntentConfig(next); err != nil {
		return intentport.ConfigView{}, err
	}
	if err := c.store.SaveConfig(ctx, next); err != nil {
		return intentport.ConfigView{}, err
	}
	c.config = next
	return next.View(), nil
}

func validateIntentConfig(config intentport.Config) error {
	switch config.Strategy {
	case intentport.StrategySystem, intentport.StrategyJev, intentport.StrategyOff:
	default:
		return errors.New("invalid intent strategy")
	}
	if config.PlanConfidence <= 0 || config.ExecuteConfidence > 1 || config.PlanConfidence > config.ExecuteConfidence {
		return errors.New("invalid intent confidence thresholds")
	}
	if config.Model == "" {
		return errors.New("Jev model is empty")
	}
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return errors.New("invalid Jev base URL")
	}
	if base.Scheme == "http" && !isLoopbackHost(base.Hostname()) {
		return errors.New("Jev API key requires HTTPS outside localhost")
	}
	if config.Strategy == intentport.StrategyJev && config.APIKey == "" {
		return errors.New("Jev API key is required")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *IntentController) Classify(ctx context.Context, current *session.Session, input string) (AutoPlanDecision, error) {
	if (current != nil && current.Kind == session.KindSubagent) || strings.TrimSpace(input) == "" {
		return AutoPlanDecision{Intent: AutoPlanIntentConversation}, nil
	}
	c.mu.RLock()
	config := c.config
	c.mu.RUnlock()
	switch config.Strategy {
	case intentport.StrategyOff:
		return AutoPlanDecision{Intent: AutoPlanIntentConversation, Reason: "automatic intent judgment is disabled"}, nil
	case intentport.StrategySystem:
		return c.builtin.Classify(ctx, current, input)
	case intentport.StrategyJev:
		if current == nil {
			return AutoPlanDecision{}, errors.New("Jev classification requires a session")
		}
		if config.APIKey == "" {
			return AutoPlanDecision{}, errors.New("Jev API key is not configured")
		}
		return c.classifyJev(ctx, current, input, config)
	default:
		return AutoPlanDecision{}, errors.New("invalid intent strategy")
	}
}

func (c *IntentController) classifyJev(ctx context.Context, current *session.Session, input string, config intentport.Config) (AutoPlanDecision, error) {
	history := recentIntentUserMessages(current)
	result, err := c.evaluateJev(ctx, current.ID, input, history, config, false)
	if err != nil {
		return AutoPlanDecision{}, err
	}
	decision := AutoPlanDecision{Intent: AutoPlanIntent(result.Choice), Confidence: result.Confidence}
	if decision.Intent == AutoPlanIntentImplementation && decision.Confidence >= config.PlanConfidence {
		decision.ShouldPlan = true
		decision.ShouldExecute = decision.Confidence >= config.ExecuteConfidence
	}
	// The trace is finalized before the caller is allowed to plan or execute.
	result.Trace.ShouldPlan = decision.ShouldPlan
	result.Trace.ShouldExecute = decision.ShouldExecute
	result.Trace.Route = "chat"
	if decision.ShouldPlan {
		result.Trace.Route = "plan_only"
		if decision.ShouldExecute {
			result.Trace.Route = "plan_execute"
		}
	}
	if err := c.store.SaveTrace(ctx, result.Trace); err != nil {
		return AutoPlanDecision{}, fmt.Errorf("record Jev decision: %w", err)
	}
	return decision, nil
}

type jevResult struct {
	Choice     string
	Confidence float64
	Trace      intentport.Trace
}

func (c *IntentController) evaluateJev(ctx context.Context, sessionID, input string, history []string, config intentport.Config, test bool) (jevResult, error) {
	runes := []rune(input)
	if len(runes) > 4000 {
		input = string(runes[:4000])
	}
	requestBody, err := c.client.BuildRequest(input, history, config.Model)
	if err != nil {
		return jevResult{}, err
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return jevResult{}, err
	}
	started := time.Now()
	trace := intentport.Trace{
		ID: hex.EncodeToString(idBytes), SessionID: sessionID, RequestID: intentport.RequestID(ctx), CreatedAt: started,
		ExpiresAt: started.Add(intentTraceRetention), BaseURL: config.BaseURL,
		RequestedModel: config.Model, RequestBody: strings.ReplaceAll(string(requestBody), config.APIKey, "[REDACTED]"), Status: "started",
	}
	if test {
		trace.Route = "test"
	}
	if err := c.store.SaveTrace(ctx, trace); err != nil {
		return jevResult{}, fmt.Errorf("record Jev request: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	responseBody, httpStatus, sendErr := c.client.Send(requestCtx, config, requestBody)
	trace.DurationMS = time.Since(started).Milliseconds()
	trace.HTTPStatus = httpStatus
	trace.ResponseBody = strings.ReplaceAll(string(responseBody), config.APIKey, "[REDACTED]")
	trace.ResponseTruncated = errors.Is(sendErr, intentport.ErrResponseTooLarge)
	if sendErr != nil {
		trace.Status = "http_error"
		trace.ErrorCode = "request_failed"
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			trace.Status, trace.ErrorCode = "timeout", "timeout"
		}
		if saveErr := c.saveFinalTrace(ctx, trace); saveErr != nil {
			return jevResult{}, fmt.Errorf("record failed Jev response: %w", saveErr)
		}
		return jevResult{}, sendErr
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		trace.Status, trace.ErrorCode = "invalid_response", "invalid_json"
		if saveErr := c.saveFinalTrace(ctx, trace); saveErr != nil {
			return jevResult{}, saveErr
		}
		return jevResult{}, errors.New("invalid Jev response JSON")
	}
	var answers map[string]json.RawMessage
	answersErr := json.Unmarshal(decoded["answers"], &answers)
	var answer struct {
		Type       string
		Choice     string
		Confidence *float64
	}
	answerErr := json.Unmarshal(answers["intent"], &answer)
	if answersErr != nil || answerErr != nil || answer.Type != "choice" || answer.Confidence == nil || *answer.Confidence < 0 || *answer.Confidence > 1 || (answer.Choice != "conversation" && answer.Choice != "explanation" && answer.Choice != "implementation") {
		trace.Status, trace.ErrorCode = "invalid_response", "invalid_choice"
		if saveErr := c.saveFinalTrace(ctx, trace); saveErr != nil {
			return jevResult{}, saveErr
		}
		return jevResult{}, errors.New("invalid Jev choice response")
	}
	var responseModel string
	_ = json.Unmarshal(decoded["model"], &responseModel)
	trace.Status, trace.Choice, trace.Confidence, trace.ResponseModel = "succeeded", answer.Choice, *answer.Confidence, responseModel
	if test {
		if err := c.saveFinalTrace(ctx, trace); err != nil {
			return jevResult{}, err
		}
	}
	return jevResult{Choice: answer.Choice, Confidence: *answer.Confidence, Trace: trace}, nil
}

func (c *IntentController) saveFinalTrace(ctx context.Context, trace intentport.Trace) error {
	// A canceled user request must not prevent recording the terminal outcome.
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return c.store.SaveTrace(auditCtx, trace)
}

func recentIntentUserMessages(current *session.Session) []string {
	if current == nil {
		return nil
	}
	history := make([]string, 0, 4)
	for i := len(current.Messages) - 1; i >= 0 && len(history) < 4; i-- {
		message := current.Messages[i]
		if message.Role != domainmessage.RoleUser || message.IsSynthetic() {
			continue
		}
		text := strings.TrimSpace(message.Text())
		if text == "" {
			continue
		}
		runes := []rune(text)
		if len(runes) > 4000 {
			text = string(runes[:4000])
		}
		history = append(history, text)
	}
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}
	return history
}

func (c *IntentController) TestJev(ctx context.Context, input intentport.Config) (int64, error) {
	c.mu.RLock()
	current := c.config
	c.mu.RUnlock()
	if input.APIKey == "" {
		input.APIKey = current.APIKey
	}
	if input.BaseURL == "" {
		input.BaseURL = current.BaseURL
	}
	if input.Model == "" {
		input.Model = current.Model
	}
	if input.PlanConfidence == 0 {
		input.PlanConfidence = current.PlanConfidence
	}
	if input.ExecuteConfidence == 0 {
		input.ExecuteConfidence = current.ExecuteConfidence
	}
	input.Strategy = intentport.StrategyJev
	if err := validateIntentConfig(input); err != nil {
		return 0, err
	}
	result, err := c.evaluateJev(ctx, "", "你好", nil, input, true)
	return result.Trace.DurationMS, err
}

func (c *IntentController) ListTraces(ctx context.Context, sessionID string, limit int) ([]intentport.Trace, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return c.store.ListTraces(ctx, sessionID, limit)
}

func (c *IntentController) GetTrace(ctx context.Context, id string) (intentport.Trace, error) {
	return c.store.GetTrace(ctx, id)
}

func (c *IntentController) ClearTraces(ctx context.Context, sessionID string) error {
	return c.store.ClearTraces(ctx, sessionID)
}
