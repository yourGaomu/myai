package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"myai/core/contextmgr"
	"myai/core/history"
	"myai/core/llm"
	"myai/core/session"
	"myai/core/store/cache"
	"myai/core/store/data"
	"myai/core/tool"
	tooldef "myai/core/tool/tool"
	"myai/utills"
)

const (
	defaultUserID      = "local"
	currentSessionTTL  = 24 * time.Hour
	maxAgentToolRounds = 6
	compactKeepChunks  = 8
)

var errNotEnoughHistoryToCompact = errors.New("not enough new history to compact")

type ChatService struct {
	client   *llm.Client
	sessions *session.SessionManage
	store    data.Store
	cache    cache.Cache
	pool     *utills.ThreadPool
	tools    *tool.RegisterTools
	modelID  string
	userID   string
}

type ContextInfo = contextmgr.Info

type CompactInfo struct {
	Triggered         bool
	BeforeTokens      int
	AfterTokens       int
	NewMessages       int
	CompactedMessages int
	SummaryTokens     int
}

type ChatResponse struct {
	SessionID string
	Result    llm.ChatResult
	Context   ContextInfo
	Compact   CompactInfo
}

func NewChatService(
	client *llm.Client,
	sessions *session.SessionManage,
	store data.Store,
	cache cache.Cache,
	pool *utills.ThreadPool,
	tools *tool.RegisterTools,
	modelID string,
) *ChatService {
	if modelID == "" {
		modelID = "gpt-5.5"
	}

	return &ChatService{
		client:   client,
		sessions: sessions,
		store:    store,
		cache:    cache,
		pool:     pool,
		tools:    tools,
		modelID:  modelID,
		userID:   defaultUserID,
	}
}

func (s *ChatService) Bootstrap(ctx context.Context) error {
	if s.sessions == nil {
		return errors.New("session manager is nil")
	}

	sessionID, err := s.cachedCurrentSession(ctx)
	if err != nil {
		return err
	}
	if sessionID != "" {
		if err := s.LoadSession(ctx, sessionID); err == nil {
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
	}

	if s.sessions.CurrentSessionId() == "" {
		return s.NewSession(ctx)
	}

	current, err := s.sessions.Current()
	if err != nil {
		return s.NewSession(ctx)
	}

	return s.saveSession(ctx, current.ID, current.Model, "New chat")
}

func (s *ChatService) SendMessage(ctx context.Context, input string) (ChatResponse, error) {
	return s.SendMessageStream(ctx, input, llm.ChatStreamHandler{})
}

func (s *ChatService) SendMessageStream(ctx context.Context, input string, stream llm.ChatStreamHandler) (ChatResponse, error) {
	return s.SendMessageStreamForSession(ctx, s.CurrentSessionID(), input, stream)
}

func (s *ChatService) SendMessageStreamForSession(ctx context.Context, sessionID string, input string, stream llm.ChatStreamHandler) (ChatResponse, error) {
	if strings.TrimSpace(input) == "" {
		return ChatResponse{}, errors.New("input is empty")
	}
	if s.client == nil {
		return ChatResponse{}, errors.New("llm client is nil")
	}
	if s.sessions == nil {
		return ChatResponse{}, errors.New("session manager is nil")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	if sessionID == "" {
		if err := s.NewSession(ctx); err != nil {
			return ChatResponse{}, err
		}
		sessionID = s.CurrentSessionID()
	}

	current, err := s.ensureSessionInMemory(ctx, sessionID, false)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := s.sessions.AddUserMessageTo(current.ID, input); err != nil {
		return ChatResponse{}, err
	}

	title := "New chat"
	if len(current.Messages) <= 2 {
		title = titleFromInput(input)
	}
	s.persistUserMessageAsync(current.ID, current.Model, title, input)
	requestID := uuid.NewString()
	taskRecorder := history.NewTaskRecorder(history.RecordOptions{
		Title:     title,
		Reason:    "user request",
		SessionID: current.ID,
		RequestID: requestID,
	})
	defer func() {
		if _, err := taskRecorder.Save(context.Background()); err != nil {
			log.Printf("save task history checkpoint failed: %v", err)
		}
		if err := taskRecorder.Close(); err != nil {
			log.Printf("close task history recorder failed: %v", err)
		}
	}()
	ctx = history.WithTaskRecorder(ctx, taskRecorder)

	model := s.client.GetModel(current.Model)
	if model == nil {
		return ChatResponse{}, fmt.Errorf("model not found: %s", current.Model)
	}

	compactInfo, err := s.autoCompactIfNeeded(ctx, current, model)
	if err != nil {
		log.Printf("auto compact failed: %v", err)
	}

	result, err := s.runAgentLoop(ctx, model, current, stream)
	if err != nil {
		return ChatResponse{}, err
	}

	if err := s.sessions.AddAssistantMessageTo(current.ID, result.Content); err != nil {
		return ChatResponse{}, err
	}
	if err := s.sessions.AddUsageTo(current.ID, result.Usage); err != nil {
		return ChatResponse{}, err
	}
	s.persistAssistantMessageAsync(sessionRecordFromSession(current, ""), result)
	s.persistCurrentSessionAsync(current.ID)

	return ChatResponse{
		SessionID: current.ID,
		Result:    result,
		Context:   contextInfoFromSession(current),
		Compact:   compactInfo,
	}, nil
}

func (s *ChatService) llmToolsForSession(current *session.Session) []llms.Tool {
	if s.tools == nil {
		return nil
	}

	mode := session.PermissionModeAsk
	if current != nil {
		mode = session.NormalizePermissionMode(current.PermissionMode)
	}

	return s.tools.LLMToolsByPermission(func(permission tooldef.Permission) bool {
		return exposeToolForPermissionMode(permission, mode)
	})
}

func (s *ChatService) runAgentLoop(ctx context.Context, model *llm.Model, current *session.Session, stream llm.ChatStreamHandler) (llm.ChatResult, error) {
	var totalUsage llm.TokenUsage
	reasoningParts := make([]string, 0, maxAgentToolRounds)

	for round := 0; round < maxAgentToolRounds; round++ {
		result, err := model.ChatWithStreamToolsHandler(s.contextMessages(current), s.llmToolsForSession(current), stream)
		if err != nil {
			return llm.ChatResult{}, err
		}

		totalUsage = totalUsage.Add(result.Usage)
		reasoningParts = appendReasoningPart(reasoningParts, result.Reasoning)
		if len(result.ToolCalls) == 0 {
			result.Usage = totalUsage
			result.Reasoning = strings.Join(reasoningParts, "\n")
			return result, nil
		}

		toolMessages, toolRecords, err := s.callTools(ctx, current, result.ToolCalls, stream)
		if err != nil {
			return llm.ChatResult{}, err
		}
		current.Messages = append(current.Messages, assistantToolCallMessage(result.ToolCalls))
		current.Messages = append(current.Messages, toolMessages...)
		s.persistToolRecordsAsync(toolRecords)
	}

	result, err := model.ChatWithStreamHandler(s.contextMessages(current), stream)
	if err != nil {
		return llm.ChatResult{}, err
	}
	totalUsage = totalUsage.Add(result.Usage)
	reasoningParts = appendReasoningPart(reasoningParts, result.Reasoning)
	result.Usage = totalUsage
	result.Reasoning = strings.Join(reasoningParts, "\n")

	return result, nil
}

func (s *ChatService) contextMessages(current *session.Session) []llms.MessageContent {
	if current == nil {
		return nil
	}
	return contextmgr.BuildWithSummary(current.Messages, current.Summary, current.CompactedMessages, current.ContextWindowK)
}

func (s *ChatService) summarizeMessages(ctx context.Context, model *llm.Model, existingSummary string, messages []llms.MessageContent) (string, error) {
	text := messagesForSummary(messages)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("no messages to compact")
	}

	prompt := `Compress the conversation history for a local coding agent.

Keep durable information only:
- User goals and preferences.
- Architecture and implementation decisions.
- Important files, tools, permissions, and configuration.
- Completed work and verification results.
- Open tasks, blockers, and next steps.
- Any safety constraints or user instructions.

Do not include secrets, API keys, or credentials.
Write a concise but useful summary in Chinese unless the source content is mostly English.`

	if strings.TrimSpace(existingSummary) != "" {
		prompt += "\n\nExisting summary:\n" + existingSummary
	}
	prompt += "\n\nNew history to compact:\n" + text

	result, err := model.ChatWithStreamHandler([]llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, "You are a context compression model for a coding assistant."),
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	}, llm.ChatStreamHandler{})
	if err != nil {
		return "", err
	}

	summary := strings.TrimSpace(result.Content)
	if summary == "" {
		return "", errors.New("compact summary is empty")
	}
	return summary, nil
}

func exposeToolForPermissionMode(permission tooldef.Permission, mode session.PermissionMode) bool {
	permission = tooldef.NormalizePermission(permission)
	mode = session.NormalizePermissionMode(mode)

	switch mode {
	case session.PermissionModeReadonly:
		return permission == tooldef.PermissionRead
	case session.PermissionModeFull:
		return true
	default:
		return true
	}
}

func appendReasoningPart(parts []string, reasoning string) []string {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return parts
	}

	return append(parts, reasoning)
}

func messagesForSummary(messages []llms.MessageContent) string {
	var builder strings.Builder
	for _, message := range messages {
		switch message.Role {
		case llms.ChatMessageTypeSystem:
			continue
		case llms.ChatMessageTypeHuman:
			writeSummaryLine(&builder, "User", messageText(message, 4000))
		case llms.ChatMessageTypeAI:
			if hasToolCallMessage(message) {
				writeSummaryLine(&builder, "Assistant tool call", messageText(message, 2000))
				continue
			}
			writeSummaryLine(&builder, "Assistant", messageText(message, 4000))
		case llms.ChatMessageTypeTool:
			writeSummaryLine(&builder, "Tool result", messageText(message, 2000))
		}
	}
	return builder.String()
}

func writeSummaryLine(builder *strings.Builder, role string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	builder.WriteString(role)
	builder.WriteString(":\n")
	builder.WriteString(text)
	builder.WriteString("\n\n")
}

func messageText(message llms.MessageContent, maxLength int) string {
	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch value := part.(type) {
		case llms.TextContent:
			parts = append(parts, value.Text)
		case llms.ToolCall:
			if value.FunctionCall == nil {
				parts = append(parts, fmt.Sprintf("tool_call id=%s", value.ID))
				continue
			}
			parts = append(parts, fmt.Sprintf("tool_call id=%s name=%s args=%s", value.ID, value.FunctionCall.Name, value.FunctionCall.Arguments))
		case llms.ToolCallResponse:
			parts = append(parts, fmt.Sprintf("tool_result id=%s name=%s content=%s", value.ToolCallID, value.Name, value.Content))
		default:
			parts = append(parts, fmt.Sprint(value))
		}
	}

	return truncateForSummary(strings.Join(parts, "\n"), maxLength)
}

func hasToolCallMessage(message llms.MessageContent) bool {
	for _, part := range message.Parts {
		if _, ok := part.(llms.ToolCall); ok {
			return true
		}
	}
	return false
}

func truncateForSummary(text string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxLength {
		return string(runes)
	}
	return string(runes[:maxLength]) + "\n[truncated]"
}

func (s *ChatService) callTools(ctx context.Context, current *session.Session, calls []llms.ToolCall, stream llm.ChatStreamHandler) ([]llms.MessageContent, []data.MessageRecord, error) {
	if s.tools == nil {
		return nil, nil, errors.New("tool registry is nil")
	}
	if current == nil {
		return nil, nil, errors.New("session is nil")
	}

	messages := make([]llms.MessageContent, 0, len(calls))
	records := make([]data.MessageRecord, 0, len(calls)*2)
	createdAt := time.Now()

	for index, call := range calls {
		if call.FunctionCall == nil {
			continue
		}

		registeredTool, err := s.tools.GetTool(call.FunctionCall.Name)
		if err != nil {
			return nil, nil, err
		}
		permission := tooldef.NormalizePermission(registeredTool.Permission())

		if stream.OnToolCall != nil {
			stream.OnToolCall(call.FunctionCall.Name, call.FunctionCall.Arguments)
		}

		records = append(records, data.MessageRecord{
			ID:            uuid.NewString(),
			SessionID:     current.ID,
			Role:          data.RoleToolCall,
			ToolCallID:    call.ID,
			ToolName:      call.FunctionCall.Name,
			ToolArguments: call.FunctionCall.Arguments,
			CreatedAt:     createdAt.Add(time.Duration(index*2) * time.Nanosecond),
		})

		result, permissionAllowed := s.allowToolCall(current, call.FunctionCall.Name, call.FunctionCall.Arguments, permission, stream)
		if permissionAllowed {
			result, err = registeredTool.Call(ctx, []byte(call.FunctionCall.Arguments))
		}
		if err != nil {
			result = "tool error: " + err.Error()
		}
		toolError := ""
		if err != nil {
			toolError = err.Error()
		}

		messages = append(messages, llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{
				llms.ToolCallResponse{
					ToolCallID: call.ID,
					Name:       call.FunctionCall.Name,
					Content:    result,
				},
			},
		})
		records = append(records, data.MessageRecord{
			ID:            uuid.NewString(),
			SessionID:     current.ID,
			Role:          data.RoleTool,
			Content:       result,
			ToolCallID:    call.ID,
			ToolName:      call.FunctionCall.Name,
			ToolArguments: call.FunctionCall.Arguments,
			ToolError:     toolError,
			CreatedAt:     createdAt.Add(time.Duration(index*2+1) * time.Nanosecond),
		})
	}

	return messages, records, nil
}

func (s *ChatService) allowToolCall(current *session.Session, name string, arguments string, permission tooldef.Permission, stream llm.ChatStreamHandler) (string, bool) {
	if permission == tooldef.PermissionRead {
		return "", true
	}

	mode := session.PermissionModeAsk
	if current != nil {
		mode = session.NormalizePermissionMode(current.PermissionMode)
	}
	switch mode {
	case session.PermissionModeReadonly:
		return fmt.Sprintf("permission denied: session permission mode is %s and tool %s requires %s", mode, name, permission), false
	case session.PermissionModeFull:
		return "", true
	default:
		if stream.OnToolAsk == nil {
			return fmt.Sprintf("permission denied: tool %s requires %s but no permission handler is configured", name, permission), false
		}
		allowed := stream.OnToolAsk(llm.ToolPermissionRequest{
			Name:       name,
			Arguments:  arguments,
			Permission: permission,
			Mode:       string(mode),
		})
		if !allowed {
			return fmt.Sprintf("permission denied by user: tool %s requires %s", name, permission), false
		}
		return "", true
	}
}

func assistantToolCallMessage(calls []llms.ToolCall) llms.MessageContent {
	parts := make([]llms.ContentPart, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, call)
	}

	return llms.MessageContent{
		Role:  llms.ChatMessageTypeAI,
		Parts: parts,
	}
}

func (s *ChatService) NewSession(ctx context.Context) error {
	if err := s.sessions.NewSession(); err != nil {
		return err
	}

	current, err := s.sessions.Current()
	if err != nil {
		return err
	}

	if err := s.saveSession(ctx, current.ID, current.Model, "New chat"); err != nil {
		return err
	}
	return s.saveCurrentSession(ctx, current.ID)
}

func (s *ChatService) LoadSession(ctx context.Context, sessionID string) error {
	current, err := s.ensureSessionInMemory(ctx, sessionID, true)
	if err != nil {
		return err
	}

	return s.saveCurrentSession(ctx, current.ID)
}

func (s *ChatService) ClearCurrent(ctx context.Context) error {
	current, err := s.sessions.Current()
	if err != nil {
		return err
	}

	if s.store != nil {
		if err := s.store.ClearMessages(ctx, current.ID); err != nil {
			return err
		}
	}

	if err := s.sessions.ClearCurrent(); err != nil {
		return err
	}

	return s.saveSession(ctx, current.ID, current.Model, "New chat")
}

func (s *ChatService) ListSessions(ctx context.Context) ([]data.SessionRecord, error) {
	if s.store == nil {
		return nil, nil
	}

	return s.store.ListSessions(ctx)
}

func (s *ChatService) ListSessionMessages(ctx context.Context, sessionID string) ([]data.MessageRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	if s.store == nil {
		return nil, nil
	}

	return s.store.ListMessages(ctx, sessionID)
}

func (s *ChatService) ListModels() []llm.ModelInfo {
	if s.client == nil {
		return nil
	}

	return s.client.ListModels()
}

func (s *ChatService) SwitchModel(ctx context.Context, modelID string) error {
	return s.SwitchModelForSession(ctx, s.CurrentSessionID(), modelID)
}

func (s *ChatService) SwitchModelForSession(ctx context.Context, sessionID string, modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return errors.New("model id is empty")
	}
	if s.client == nil {
		return errors.New("llm client is nil")
	}
	if !s.client.HasModel(modelID) {
		return fmt.Errorf("model not found: %s", modelID)
	}
	if s.sessions == nil {
		return errors.New("session manager is nil")
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return s.sessions.SwitchModel(modelID)
	}

	current, err := s.ensureSessionInMemory(ctx, sessionID, false)
	if err != nil {
		return err
	}

	if err := s.sessions.SwitchModelForSession(current.ID, modelID); err != nil {
		return err
	}
	current.Model = modelID

	return s.saveSession(ctx, current.ID, current.Model, "")
}

func (s *ChatService) SetPermissionMode(ctx context.Context, mode string) error {
	return s.SetPermissionModeForSession(ctx, s.CurrentSessionID(), mode)
}

func (s *ChatService) SetPermissionModeForSession(ctx context.Context, sessionID string, mode string) error {
	if s.sessions == nil {
		return errors.New("session manager is nil")
	}

	permissionMode := session.PermissionMode(strings.TrimSpace(mode))
	if !session.IsPermissionMode(permissionMode) {
		return fmt.Errorf("unsupported permission mode: %s", mode)
	}

	current, err := s.ensureSessionInMemory(ctx, sessionID, false)
	if err != nil {
		return err
	}

	if err := s.sessions.SetPermissionModeForSession(current.ID, permissionMode); err != nil {
		return err
	}
	current.PermissionMode = permissionMode

	return s.saveSession(ctx, current.ID, current.Model, "")
}

func (s *ChatService) SetContextWindowK(ctx context.Context, windowK int) error {
	return s.SetContextWindowKForSession(ctx, s.CurrentSessionID(), windowK)
}

func (s *ChatService) SetContextWindowKForSession(ctx context.Context, sessionID string, windowK int) error {
	if s.sessions == nil {
		return errors.New("session manager is nil")
	}
	if err := contextmgr.ValidateWindowK(windowK); err != nil {
		return err
	}

	current, err := s.ensureSessionInMemory(ctx, sessionID, false)
	if err != nil {
		return err
	}

	if err := s.sessions.SetContextWindowKForSession(current.ID, windowK); err != nil {
		return err
	}
	current.ContextWindowK = contextmgr.NormalizeWindowK(windowK)

	return s.saveSession(ctx, current.ID, current.Model, "")
}

func (s *ChatService) CompactCurrentSession(ctx context.Context) (ContextInfo, error) {
	return s.CompactSession(ctx, s.CurrentSessionID())
}

func (s *ChatService) CompactSession(ctx context.Context, sessionID string) (ContextInfo, error) {
	if s.sessions == nil {
		return ContextInfo{}, errors.New("session manager is nil")
	}
	if s.client == nil {
		return ContextInfo{}, errors.New("llm client is nil")
	}

	current, err := s.ensureSessionInMemory(ctx, sessionID, false)
	if err != nil {
		return ContextInfo{}, err
	}

	model := s.client.GetModel(current.Model)
	if model == nil {
		return ContextInfo{}, fmt.Errorf("model not found: %s", current.Model)
	}

	if err := s.compactSession(ctx, current, model); err != nil {
		if errors.Is(err, errNotEnoughHistoryToCompact) {
			return contextInfoFromSession(current), err
		}
		return ContextInfo{}, err
	}

	return contextInfoFromSession(current), nil
}

func (s *ChatService) autoCompactIfNeeded(ctx context.Context, current *session.Session, model *llm.Model) (CompactInfo, error) {
	if current == nil || model == nil {
		return CompactInfo{}, nil
	}

	before, _ := contextmgr.AnalyzeWithSummary(current.Messages, current.Summary, current.CompactedMessages, current.ContextWindowK)
	if !before.Truncated {
		return CompactInfo{}, nil
	}

	if err := s.compactSession(ctx, current, model); errors.Is(err, errNotEnoughHistoryToCompact) {
		return CompactInfo{}, nil
	} else {
		after, _ := contextmgr.AnalyzeWithSummary(current.Messages, current.Summary, current.CompactedMessages, current.ContextWindowK)
		return CompactInfo{
			Triggered:         true,
			BeforeTokens:      before.SelectedTokens,
			AfterTokens:       after.SelectedTokens,
			NewMessages:       after.CompactedMessages - before.CompactedMessages,
			CompactedMessages: after.CompactedMessages,
			SummaryTokens:     after.SummaryTokens,
		}, err
	}
}

func (s *ChatService) compactSession(ctx context.Context, current *session.Session, model *llm.Model) error {
	compactable, _, cutoff := contextmgr.CompactSplit(current.Messages, current.CompactedMessages, compactKeepChunks)
	if len(compactable) == 0 {
		return errNotEnoughHistoryToCompact
	}

	summary, err := s.summarizeMessages(ctx, model, current.Summary, compactable)
	if err != nil {
		return err
	}
	if err := s.sessions.SetSummaryForSession(current.ID, summary, cutoff); err != nil {
		return err
	}
	if err := s.saveSession(ctx, current.ID, current.Model, ""); err != nil {
		return err
	}

	return nil
}

func (s *ChatService) AddModelConfig(ctx context.Context, config data.ModelConfig) error {
	if s.store == nil {
		return errors.New("model store is nil")
	}
	if s.client == nil {
		return errors.New("llm client is nil")
	}

	config.ID = strings.TrimSpace(config.ID)
	config.Name = strings.TrimSpace(config.Name)
	config.Provider = strings.TrimSpace(strings.ToLower(config.Provider))
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.ModelName = strings.TrimSpace(config.ModelName)

	if config.ID == "" {
		return errors.New("model id is empty")
	}
	if strings.ContainsAny(config.ID, " \t\r\n") {
		return errors.New("model id cannot contain spaces")
	}
	if s.client.HasModel(config.ID) {
		return fmt.Errorf("model already exists: %s", config.ID)
	}
	if config.Name == "" {
		config.Name = config.ID
	}
	if config.Provider == "" {
		config.Provider = "openai"
	}
	if config.Provider != "openai" && config.Provider != "openai-compatible" {
		return fmt.Errorf("unsupported provider: %s", config.Provider)
	}
	if config.BaseURL == "" {
		return errors.New("base url is empty")
	}
	if config.APIKey == "" {
		return errors.New("api key is empty")
	}
	if config.ModelName == "" {
		config.ModelName = config.ID
	}

	model, err := utills.CreateLLM(config.APIKey, config.BaseURL, config.ModelName)
	if err != nil {
		return err
	}

	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	config.Enabled = true

	if err := s.store.SaveModelConfig(ctx, config); err != nil {
		return err
	}

	s.client.SetModelInfo(config.ID, model, llm.ModelInfo{
		ID:        config.ID,
		Name:      config.Name,
		Provider:  config.Provider,
		ModelName: config.ModelName,
		Enabled:   config.Enabled,
		IsDefault: config.IsDefault,
	})
	return nil
}

func (s *ChatService) CurrentSessionID() string {
	if s.sessions == nil {
		return ""
	}
	return s.sessions.CurrentSessionId()
}

func (s *ChatService) CurrentModelID() string {
	if s.sessions == nil {
		return s.modelID
	}

	modelID := s.sessions.CurrentModelId()
	if modelID == "" {
		return s.modelID
	}
	return modelID
}

func (s *ChatService) CurrentPermissionMode() session.PermissionMode {
	if s.sessions == nil {
		return session.PermissionModeAsk
	}
	return s.sessions.CurrentPermissionMode()
}

func (s *ChatService) CurrentContextWindowK() int {
	if s.sessions == nil {
		return contextmgr.DefaultWindowK
	}
	return contextmgr.NormalizeWindowK(s.sessions.CurrentContextWindowK())
}

func (s *ChatService) CurrentUsage() llm.TokenUsage {
	if s.sessions == nil {
		return llm.TokenUsage{}
	}
	return s.sessions.CurrentUsage()
}

func (s *ChatService) CurrentLastUsage() llm.TokenUsage {
	if s.sessions == nil {
		return llm.TokenUsage{}
	}
	return s.sessions.CurrentLastUsage()
}

func (s *ChatService) CurrentContextInfo() ContextInfo {
	if s.sessions == nil {
		return ContextInfo{WindowK: contextmgr.DefaultWindowK}
	}

	current, err := s.sessions.Current()
	if err != nil {
		return ContextInfo{WindowK: contextmgr.DefaultWindowK}
	}

	return contextInfoFromSession(current)
}

func (s *ChatService) ContextInfoForSession(ctx context.Context, sessionID string) (ContextInfo, error) {
	if s.sessions == nil {
		return ContextInfo{WindowK: contextmgr.DefaultWindowK}, errors.New("session manager is nil")
	}

	current, err := s.ensureSessionInMemory(ctx, sessionID, false)
	if err != nil {
		return ContextInfo{WindowK: contextmgr.DefaultWindowK}, err
	}
	return contextInfoFromSession(current), nil
}

func (s *ChatService) saveSession(ctx context.Context, sessionID string, model string, title string) error {
	if s.store == nil {
		return nil
	}
	if model == "" {
		model = s.modelID
	}
	if title == "" {
		title = "New chat"
	}
	permissionMode := string(session.PermissionModeAsk)
	contextWindowK := contextmgr.DefaultWindowK
	summary := ""
	compactedMessages := 0
	var compactedAt *time.Time
	var usage *data.TokenUsageRecord
	var lastUsage *data.TokenUsageRecord
	hasCurrentState := false
	if current, err := s.sessions.GetSession(sessionID); err == nil && current != nil && current.ID == sessionID {
		hasCurrentState = true
		permissionMode = string(session.NormalizePermissionMode(current.PermissionMode))
		contextWindowK = contextmgr.NormalizeWindowK(current.ContextWindowK)
		summary = current.Summary
		compactedMessages = current.CompactedMessages
		usage = tokenUsageRecord(current.Usage)
		lastUsage = tokenUsageRecord(current.LastUsage)
		if summary != "" {
			now := time.Now()
			compactedAt = &now
		}
	}

	if !hasCurrentState {
		record, err := s.getSession(ctx, sessionID)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if record.PermissionMode != "" {
			permissionMode = record.PermissionMode
		}
		if record.ContextWindowK > 0 {
			contextWindowK = record.ContextWindowK
		}
		summary = record.Summary
		compactedMessages = record.CompactedMessages
		compactedAt = record.CompactedAt
		usage = record.Usage
		lastUsage = record.LastUsage
	}

	return s.saveSessionRecord(ctx, data.SessionRecord{
		ID:                sessionID,
		Model:             model,
		PermissionMode:    permissionMode,
		ContextWindowK:    contextWindowK,
		Summary:           summary,
		CompactedMessages: compactedMessages,
		CompactedAt:       compactedAt,
		Title:             title,
		Usage:             usage,
		LastUsage:         lastUsage,
	})
}

func (s *ChatService) saveSessionRecord(ctx context.Context, record data.SessionRecord) error {
	if s.store == nil {
		return nil
	}
	if record.ID == "" {
		return errors.New("session id is empty")
	}
	if record.Model == "" {
		record.Model = s.modelID
	}
	if record.PermissionMode == "" {
		record.PermissionMode = string(session.PermissionModeAsk)
	}
	record.ContextWindowK = contextmgr.NormalizeWindowK(record.ContextWindowK)
	if record.Title == "" {
		record.Title = "New chat"
	}

	now := time.Now()
	existing, err := s.getSession(ctx, record.ID)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = existing.CreatedAt
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if existing.Title != "" && existing.Title != "New chat" {
		record.Title = existing.Title
	}
	record.UpdatedAt = now

	return s.store.SaveSession(ctx, record)
}

func (s *ChatService) ensureSessionInMemory(ctx context.Context, sessionID string, setCurrent bool) (*session.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	if s.sessions == nil {
		return nil, errors.New("session manager is nil")
	}

	current, err := s.sessions.GetSession(sessionID)
	if err == nil {
		if setCurrent {
			if err := s.sessions.UseSession(sessionID); err != nil {
				return nil, err
			}
		}
		return current, nil
	}

	record, err := s.getSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages, err := s.messagesFromStore(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	put := s.sessions.PutSessionWithUsageNoCurrent
	if setCurrent {
		put = s.sessions.PutSessionWithUsage
	}
	if err := put(
		record.ID,
		record.Model,
		session.PermissionMode(record.PermissionMode),
		record.ContextWindowK,
		record.Summary,
		record.CompactedMessages,
		tokenUsageFromRecord(record.Usage),
		tokenUsageFromRecord(record.LastUsage),
		messages,
	); err != nil {
		return nil, err
	}

	return s.sessions.GetSession(record.ID)
}

func contextInfoFromSession(current *session.Session) ContextInfo {
	if current == nil {
		return ContextInfo{WindowK: contextmgr.DefaultWindowK}
	}

	info, _ := contextmgr.AnalyzeWithSummary(current.Messages, current.Summary, current.CompactedMessages, current.ContextWindowK)
	return info
}

func sessionRecordFromSession(current *session.Session, title string) data.SessionRecord {
	if current == nil {
		return data.SessionRecord{}
	}

	return data.SessionRecord{
		ID:                current.ID,
		Model:             current.Model,
		PermissionMode:    string(session.NormalizePermissionMode(current.PermissionMode)),
		ContextWindowK:    contextmgr.NormalizeWindowK(current.ContextWindowK),
		Summary:           current.Summary,
		CompactedMessages: current.CompactedMessages,
		Title:             title,
		Usage:             tokenUsageRecord(current.Usage),
		LastUsage:         tokenUsageRecord(current.LastUsage),
	}
}

func tokenUsageRecord(usage llm.TokenUsage) *data.TokenUsageRecord {
	if !usage.Available &&
		usage.PromptTokens == 0 &&
		usage.CompletionTokens == 0 &&
		usage.TotalTokens == 0 &&
		usage.ReasoningTokens == 0 &&
		usage.PromptCachedTokens == 0 {
		return nil
	}

	return &data.TokenUsageRecord{
		PromptTokens:       usage.PromptTokens,
		CompletionTokens:   usage.CompletionTokens,
		TotalTokens:        usage.TotalTokens,
		ReasoningTokens:    usage.ReasoningTokens,
		PromptCachedTokens: usage.PromptCachedTokens,
		Available:          usage.Available,
	}
}

func tokenUsageFromRecord(record *data.TokenUsageRecord) llm.TokenUsage {
	if record == nil {
		return llm.TokenUsage{}
	}

	return llm.TokenUsage{
		PromptTokens:       record.PromptTokens,
		CompletionTokens:   record.CompletionTokens,
		TotalTokens:        record.TotalTokens,
		ReasoningTokens:    record.ReasoningTokens,
		PromptCachedTokens: record.PromptCachedTokens,
		Available:          record.Available,
	}
}

func (s *ChatService) saveAssistantMessage(ctx context.Context, sessionID string, result llm.ChatResult) error {
	return s.saveMessage(ctx, data.MessageRecord{
		ID:                 uuid.NewString(),
		SessionID:          sessionID,
		Role:               data.RoleAssistant,
		Content:            result.Content,
		Reasoning:          result.Reasoning,
		PromptTokens:       result.Usage.PromptTokens,
		CompletionTokens:   result.Usage.CompletionTokens,
		TotalTokens:        result.Usage.TotalTokens,
		ReasoningTokens:    result.Usage.ReasoningTokens,
		PromptCachedTokens: result.Usage.PromptCachedTokens,
		CreatedAt:          time.Now(),
	})
}

func (s *ChatService) persistUserMessageAsync(sessionID string, model string, title string, input string) {
	createdAt := time.Now()

	s.runAsync(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.saveSession(ctx, sessionID, model, title); err != nil {
			log.Printf("save session failed: %v", err)
		}
		if err := s.saveMessage(ctx, data.MessageRecord{
			ID:        uuid.NewString(),
			SessionID: sessionID,
			Role:      data.RoleUser,
			Content:   input,
			CreatedAt: createdAt,
		}); err != nil {
			log.Printf("save user message failed: %v", err)
		}
	})
}

func (s *ChatService) persistAssistantMessageAsync(sessionRecord data.SessionRecord, result llm.ChatResult) {
	s.runAsync(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.saveAssistantMessage(ctx, sessionRecord.ID, result); err != nil {
			log.Printf("save assistant message failed: %v", err)
		}
		if err := s.saveSessionRecord(ctx, sessionRecord); err != nil {
			log.Printf("save assistant session failed: %v", err)
		}
	})
}

func (s *ChatService) persistToolRecordsAsync(records []data.MessageRecord) {
	if len(records) == 0 {
		return
	}

	s.runAsync(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, record := range records {
			if err := s.saveMessage(ctx, record); err != nil {
				log.Printf("save tool record failed: %v", err)
			}
		}
	})
}

func (s *ChatService) persistCurrentSessionAsync(sessionID string) {
	s.runAsync(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.saveCurrentSession(ctx, sessionID); err != nil {
			log.Printf("save current session failed: %v", err)
		}
	})
}

func (s *ChatService) runAsync(task func()) {
	if task == nil {
		return
	}
	if s.pool == nil {
		go task()
		return
	}
	if err := s.pool.Submit(task); err != nil {
		go task()
	}
}

func (s *ChatService) saveMessage(ctx context.Context, message data.MessageRecord) error {
	if s.store == nil {
		return nil
	}

	return s.store.SaveMessage(ctx, message)
}

func (s *ChatService) getSession(ctx context.Context, sessionID string) (data.SessionRecord, error) {
	if s.store == nil {
		return data.SessionRecord{}, mongo.ErrNoDocuments
	}

	return s.store.GetSession(ctx, sessionID)
}

func (s *ChatService) cachedCurrentSession(ctx context.Context) (string, error) {
	if s.cache == nil {
		return "", nil
	}

	return s.cache.GetCurrentSession(ctx, s.userID)
}

func (s *ChatService) saveCurrentSession(ctx context.Context, sessionID string) error {
	if s.cache == nil {
		return nil
	}

	return s.cache.SetCurrentSession(ctx, s.userID, sessionID, currentSessionTTL)
}

func (s *ChatService) messagesFromStore(ctx context.Context, sessionID string) ([]llms.MessageContent, error) {
	if s.store == nil {
		return nil, nil
	}

	records, err := s.store.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages := make([]llms.MessageContent, 0, len(records)+1)
	messages = append(messages, llms.TextParts(llms.ChatMessageTypeSystem, session.SystemPrompt()))
	for _, record := range records {
		switch record.Role {
		case data.RoleUser:
			messages = append(messages, llms.TextParts(llms.ChatMessageTypeHuman, record.Content))
		case data.RoleAssistant:
			messages = append(messages, llms.TextParts(llms.ChatMessageTypeAI, record.Content))
		case data.RoleToolCall:
			messages = append(messages, llms.MessageContent{
				Role: llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{
					llms.ToolCall{
						ID:   record.ToolCallID,
						Type: "function",
						FunctionCall: &llms.FunctionCall{
							Name:      record.ToolName,
							Arguments: record.ToolArguments,
						},
					},
				},
			})
		case data.RoleTool:
			messages = append(messages, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: record.ToolCallID,
						Name:       record.ToolName,
						Content:    record.Content,
					},
				},
			})
		}
	}

	return messages, nil
}

func titleFromInput(input string) string {
	text := strings.TrimSpace(input)
	if text == "" {
		return "New chat"
	}

	const maxTitleLength = 30
	if len([]rune(text)) <= maxTitleLength {
		return text
	}

	runes := []rune(text)
	return string(runes[:maxTitleLength]) + "..."
}
