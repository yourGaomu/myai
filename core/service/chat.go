package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	agentrunquery "myai/core/application/agentrun/query"
	agentrunresult "myai/core/application/agentrun/result"
	compactioncommand "myai/core/application/chat/compaction/command"
	compactionresult "myai/core/application/chat/compaction/result"
	generationcommand "myai/core/application/chat/generation/command"
	plancommand "myai/core/application/chat/plan/command"
	planport "myai/core/application/chat/plan/port"
	chatretrievalcommand "myai/core/application/chat/retrieval/command"
	chatretrievalresult "myai/core/application/chat/retrieval/result"
	modelcommand "myai/core/application/model/command"
	bootstrapcommand "myai/core/application/session/bootstrap/command"
	bootstrapresult "myai/core/application/session/bootstrap/result"
	lifecyclecommand "myai/core/application/session/lifecycle/command"
	loadcommand "myai/core/application/session/load/command"
	messagecommand "myai/core/application/session/message/command"
	querycommand "myai/core/application/session/query/command"
	sessionresult "myai/core/application/session/result"
	settingscommand "myai/core/application/session/settings/command"
	skillquery "myai/core/application/skill/query"
	"myai/core/contextmgr"
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/session"
	"myai/core/skill"
)

type ChatService struct {
	// ChatService 是 CLI 与远程 Agent 共用的 Facade；业务实现由 dependencies 中的应用用例完成。
	dependencies  ChatDependencies
	operationInit sync.Once
	operations    *sessionOperationCoordinator
}

type ContextInfo = contextmgr.Info

type ContextState struct {
	Info    ContextInfo
	Summary string
}

type CompactInfo = compactionresult.CompactInfo

type ChatResponse struct {
	SessionID string
	Result    llm.ChatResult
	Context   ContextInfo
	Compact   CompactInfo
	Plan      *agentplan.Plan
	Retrieval chatretrievalresult.Context
}

type SessionPreferencesView struct {
	SessionOverrides generation.Settings
	ModelDefaults    generation.Settings
	Effective        generation.ResolvedSettings
	StyleInstruction string
	RAGSettings      session.RAGSettings
}

func NewChatService(dependencies ChatDependencies) *ChatService {
	return &ChatService{dependencies: dependencies}
}

func (s *ChatService) Bootstrap(ctx context.Context) error {
	// 进程启动时优先恢复上次当前会话；没有可恢复会话时再创建新会话。
	result, err := s.dependencies.SessionBootstrap.Bootstrap(ctx, bootstrapcommand.Bootstrap{
		NewSessionTitle: "New chat",
	})
	if err != nil {
		return err
	}
	switch result.Action {
	case bootstrapresult.ActionLoaded:
		s.emitSessionChangedHook(ctx, result.Session.ID, "load")
	case bootstrapresult.ActionCreated:
		s.emitSessionChangedHook(ctx, result.Session.ID, "new")
	}
	return nil
}

func (s *ChatService) SendMessage(ctx context.Context, input string) (ChatResponse, error) {
	return s.SendMessageStream(ctx, input, llm.ChatStreamHandler{})
}

func (s *ChatService) SendMessageStream(ctx context.Context, input string, stream llm.ChatStreamHandler) (ChatResponse, error) {
	return s.SendMessageStreamForSession(ctx, s.CurrentSessionID(), input, stream)
}

func (s *ChatService) SendMessageStreamForSession(ctx context.Context, sessionID string, input string, stream llm.ChatStreamHandler) (ChatResponse, error) {
	if s.dependencies.Models == nil {
		return ChatResponse{}, errors.New("llm client is nil")
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
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return ChatResponse{}, err
	}
	defer unlock()

	// RAG Context 与运行时指令都位于本轮消息尾部，不改变固定 System Prompt 和历史缓存前缀。
	var retrievalInfo chatretrievalresult.Context
	if s.dependencies.RetrievalContext != nil {
		current, loadErr := s.dependencies.SessionLoader.Load(ctx, sessionID)
		if loadErr != nil {
			return ChatResponse{}, loadErr
		}
		var retrievalErr error
		retrievalInfo, retrievalErr = s.dependencies.RetrievalContext.Prepare(ctx, chatretrievalcommand.Prepare{Session: current, Input: input})
		if retrievalErr != nil {
			// 自动检索失败不阻断聊天；错误随终态响应返回客户端。
			retrievalInfo.Error = retrievalErr.Error()
		}
	}
	prepared, err := s.dependencies.MessageCommands.AppendUserMessage(ctx, messagecommand.AppendUserMessage{
		SessionID:  sessionID,
		Input:      input,
		RAGContext: retrievalInfo.Prompt,
	})
	if err != nil {
		return ChatResponse{}, err
	}
	current := prepared.Session

	title := "New chat"
	if userMessageCount(current.Messages) == 1 {
		title = titleFromInput(input)
	}
	// 持久化与主生成流程解耦；落库失败由适配器上报，不阻断已经开始的模型请求。
	if s.dependencies.UserMessages != nil {
		s.dependencies.UserMessages.PersistUserMessage(generationcommand.PersistUserMessage{
			SessionID:          current.ID,
			Model:              current.Model,
			Title:              title,
			Input:              input,
			RuntimeInstruction: prepared.RuntimeInstruction,
			RAGContext:         prepared.RAGContext,
			SessionSnapshot:    session.Clone(current),
		})
	}

	return s.generateAssistantForSession(ctx, current, input, title, "user request", retrievalInfo, stream, true, false)
}

func (s *ChatService) ContinueSessionStreamForSession(ctx context.Context, sessionID string, input string, syntheticReason domainmessage.SyntheticReason, stream llm.ChatStreamHandler) (ChatResponse, error) {
	if s.dependencies.Models == nil {
		return ChatResponse{}, errors.New("llm client is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ChatResponse{}, errors.New("session id is empty")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return ChatResponse{}, errors.New("continuation input is empty")
	}
	if syntheticReason == "" {
		return ChatResponse{}, errors.New("continuation synthetic reason is empty")
	}

	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return ChatResponse{}, err
	}
	defer unlock()

	prepared, err := s.dependencies.MessageCommands.AppendUserMessage(ctx, messagecommand.AppendUserMessage{
		SessionID: sessionID, Input: input, ForceChatMode: true, SyntheticReason: syntheticReason,
		DeduplicateSynthetic: true,
	})
	if err != nil {
		return ChatResponse{}, err
	}
	current := prepared.Session
	if prepared.Appended && s.dependencies.UserMessages != nil {
		s.dependencies.UserMessages.PersistUserMessage(generationcommand.PersistUserMessage{
			SessionID: current.ID, Model: current.Model, Input: input,
			RuntimeInstruction: prepared.RuntimeInstruction, RAGContext: prepared.RAGContext,
			SyntheticReason: syntheticReason, SessionSnapshot: session.Clone(current),
		})
	}

	return s.generateAssistantForSession(
		ctx, current, input, "", "resume parent session from background subagent",
		chatretrievalresult.Context{}, stream, false, true,
	)
}

func userMessageCount(messages []domainmessage.Message) int {
	count := 0
	for _, message := range messages {
		if message.Role == domainmessage.RoleUser {
			count++
		}
	}
	return count
}

func (s *ChatService) RegenerateLastMessageStreamForSession(ctx context.Context, sessionID string, stream llm.ChatStreamHandler) (ChatResponse, error) {
	if s.dependencies.Models == nil {
		return ChatResponse{}, errors.New("llm client is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return ChatResponse{}, err
	}
	defer unlock()

	prepared, err := s.dependencies.MessageCommands.PrepareRegeneration(ctx, messagecommand.PrepareRegeneration{
		SessionID: sessionID,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	return s.generateAssistantForSession(ctx, prepared.Session, prepared.Input, "", "regenerate response", chatretrievalresult.Context{}, stream, true, false)
}

func (s *ChatService) ExecutePlanStreamForSession(ctx context.Context, sessionID string, stream llm.ChatStreamHandler, onPlanUpdate func(*agentplan.Plan)) (ChatResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return ChatResponse{}, err
	}
	defer unlock()

	// UpdateSink 把步骤 running/done/failed 状态实时桥接到远程协议层。
	var updates planport.UpdateSink
	if onPlanUpdate != nil {
		updates = planUpdateSinkFunc(onPlanUpdate)
	}
	result, err := s.dependencies.PlanExecution.Execute(ctx, plancommand.Execute{
		SessionID: sessionID,
		Stream:    stream,
	}, updates)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		SessionID: result.SessionID,
		Result:    result.Result,
		Context:   result.Context,
		Compact:   result.Compact,
		Plan:      result.Plan,
	}, nil
}

type planUpdateSinkFunc func(currentPlan *agentplan.Plan)

func (f planUpdateSinkFunc) PlanUpdated(currentPlan *agentplan.Plan) {
	f(currentPlan)
}

func (s *ChatService) generateAssistantForSession(ctx context.Context, current *session.Session, latestInput string, title string, reason string, retrievalInfo chatretrievalresult.Context, stream llm.ChatStreamHandler, capturePlan bool, forceChatMode bool) (ChatResponse, error) {
	// Normal chat can capture a Plan; internal continuations force Chat mode and skip Plan parsing.
	response, err := s.dependencies.GenerationTasks.Generate(ctx, generationcommand.GenerationTask{
		Session:       current,
		LatestInput:   latestInput,
		Title:         title,
		Reason:        reason,
		Stream:        stream,
		CapturePlan:   capturePlan,
		ForceChatMode: forceChatMode,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	return ChatResponse{
		SessionID: response.SessionID,
		Result:    response.Result,
		Context:   response.Context,
		Compact:   response.Compact,
		Plan:      response.Plan,
		Retrieval: retrievalInfo,
	}, nil
}

func (s *ChatService) contextInfo(ctx context.Context, current *session.Session) ContextInfo {
	return s.dependencies.ContextQueries.Info(ctx, current)
}

func (s *ChatService) NewSession(ctx context.Context) error {
	unlock, err := s.lockSessionOperation(ctx, "")
	if err != nil {
		return err
	}
	defer unlock()
	_, err = s.dependencies.SessionLifecycle.Create(ctx, lifecyclecommand.CreateSession{Title: "New chat"})
	return err
}

func (s *ChatService) LoadSession(ctx context.Context, sessionID string) error {
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = s.dependencies.SessionLifecycle.Load(ctx, lifecyclecommand.LoadSession{SessionID: sessionID})
	return err
}

func (s *ChatService) DeleteSession(ctx context.Context, sessionID string) error {
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = s.dependencies.SessionLifecycle.Delete(ctx, lifecyclecommand.DeleteSession{SessionID: sessionID})
	return err
}

func (s *ChatService) RestoreSession(ctx context.Context, sessionID string) error {
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = s.dependencies.SessionLifecycle.Restore(ctx, lifecyclecommand.RestoreSession{SessionID: sessionID})
	return err
}

func (s *ChatService) ClearCurrent(ctx context.Context) error {
	unlock, err := s.lockSessionOperation(ctx, s.CurrentSessionID())
	if err != nil {
		return err
	}
	defer unlock()
	_, err = s.dependencies.SessionLifecycle.Clear(ctx, lifecyclecommand.ClearSession{Title: "New chat"})
	return err
}

func (s *ChatService) ListSessions(ctx context.Context) ([]sessionresult.SessionListItem, error) {
	return s.ListSessionsWithDeleted(ctx, false)
}

func (s *ChatService) ListDeletedSessions(ctx context.Context) ([]sessionresult.SessionListItem, error) {
	return s.ListSessionsWithDeleted(ctx, true)
}

func (s *ChatService) ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]sessionresult.SessionListItem, error) {
	return s.dependencies.SessionQueries.ListSessions(ctx, includeDeleted)
}

func (s *ChatService) ListSessionMessages(ctx context.Context, sessionID string) ([]sessionresult.MessageListItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	return s.dependencies.MessageQueries.ListMessages(ctx, sessionID)
}

func (s *ChatService) ListAgentRuns(ctx context.Context, sessionID string, limit int) ([]agentrunresult.Snapshot, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	if s.dependencies.AgentRunQueries == nil {
		return []agentrunresult.Snapshot{}, nil
	}
	return s.dependencies.AgentRunQueries.ListSessionRuns(ctx, agentrunquery.ListSessionRuns{
		SessionID: sessionID,
		Limit:     limit,
	})
}

func (s *ChatService) SessionHistoryMeta(ctx context.Context, sessionID string) (sessionresult.MessageHistoryMeta, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	if sessionID == "" {
		return sessionresult.MessageHistoryMeta{}, errors.New("session id is empty")
	}
	return s.dependencies.MessageQueries.HistoryMeta(ctx, sessionID)
}

func (s *ChatService) ListSessionMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]sessionresult.MessageListItem, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	if sessionID == "" {
		return nil, false, errors.New("session id is empty")
	}
	return s.dependencies.MessageQueries.ListMessagesAfter(ctx, sessionID, strings.TrimSpace(afterMessageID), limit)
}

func (s *ChatService) ListAssets(ctx context.Context, sessionID string, limit int) ([]sessionresult.AssetListItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.CurrentSessionID()
	}
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	return s.dependencies.SessionQueries.ListAssets(ctx, querycommand.ListAssets{
		SessionID: sessionID,
		Limit:     limit,
	})
}

func (s *ChatService) ListModels() []llm.ModelInfo {
	return s.dependencies.ModelQueries.ListModels().Models
}

func (s *ChatService) ListSkills(ctx context.Context) ([]skill.Skill, error) {
	result, err := s.dependencies.SkillCatalog.List(ctx, skillquery.ListSkills{Refresh: true})
	return result.Skills, err
}

func (s *ChatService) ReloadSkills(ctx context.Context, reason string) ([]skill.Skill, error) {
	skills, err := s.ListSkills(ctx)
	if err != nil {
		return nil, err
	}
	s.emitSkillReloadedHook(ctx, len(skills), reason)
	return skills, nil
}

func (s *ChatService) SkillRoot() string {
	return s.dependencies.SkillCatalog.Root()
}

func (s *ChatService) SwitchModel(ctx context.Context, modelID string) error {
	return s.SwitchModelForSession(ctx, s.CurrentSessionID(), modelID)
}

func (s *ChatService) SwitchModelForSession(ctx context.Context, sessionID string, modelID string) error {
	return s.withSessionOperation(ctx, sessionID, func() error {
		return s.dependencies.SessionSettings.SwitchModel(ctx, settingscommand.SwitchModel{
			SessionID: sessionID,
			ModelID:   modelID,
		})
	})
}

func (s *ChatService) SetPermissionMode(ctx context.Context, mode string) error {
	return s.SetPermissionModeForSession(ctx, s.CurrentSessionID(), mode)
}

func (s *ChatService) SetPermissionModeForSession(ctx context.Context, sessionID string, mode string) error {
	return s.withSessionOperation(ctx, sessionID, func() error {
		return s.dependencies.SessionSettings.SetPermissionMode(ctx, settingscommand.SetPermissionMode{
			SessionID: sessionID,
			Mode:      mode,
		})
	})
}

func (s *ChatService) SetAgentMode(ctx context.Context, mode string) error {
	return s.SetAgentModeForSession(ctx, s.CurrentSessionID(), mode)
}

func (s *ChatService) SetAgentModeForSession(ctx context.Context, sessionID string, mode string) error {
	return s.withSessionOperation(ctx, sessionID, func() error {
		return s.dependencies.SessionSettings.SetAgentMode(ctx, settingscommand.SetAgentMode{
			SessionID: sessionID,
			Mode:      mode,
		})
	})
}

func (s *ChatService) SetContextWindowK(ctx context.Context, windowK int) error {
	return s.SetContextWindowKForSession(ctx, s.CurrentSessionID(), windowK)
}

func (s *ChatService) SetContextWindowKForSession(ctx context.Context, sessionID string, windowK int) error {
	return s.withSessionOperation(ctx, sessionID, func() error {
		return s.dependencies.SessionSettings.SetContextWindow(ctx, settingscommand.SetContextWindow{
			SessionID: sessionID,
			WindowK:   windowK,
		})
	})
}

func (s *ChatService) SetGenerationSettings(ctx context.Context, settings generation.Settings) error {
	return s.SetGenerationSettingsForSession(ctx, s.CurrentSessionID(), settings)
}

func (s *ChatService) SetGenerationSettingsForSession(ctx context.Context, sessionID string, settings generation.Settings) error {
	return s.withSessionOperation(ctx, sessionID, func() error {
		return s.dependencies.SessionSettings.SetGenerationSettings(ctx, settingscommand.SetGenerationSettings{
			SessionID: sessionID,
			Settings:  settings,
		})
	})
}

func (s *ChatService) SetStyleInstruction(ctx context.Context, instruction string) error {
	return s.SetStyleInstructionForSession(ctx, s.CurrentSessionID(), instruction)
}

func (s *ChatService) SetStyleInstructionForSession(ctx context.Context, sessionID string, instruction string) error {
	return s.withSessionOperation(ctx, sessionID, func() error {
		return s.dependencies.SessionSettings.SetStyleInstruction(ctx, settingscommand.SetStyleInstruction{
			SessionID:   sessionID,
			Instruction: instruction,
		})
	})
}

func (s *ChatService) SetRAGSettings(ctx context.Context, settings session.RAGSettings) error {
	return s.SetRAGSettingsForSession(ctx, s.CurrentSessionID(), settings)
}

func (s *ChatService) SetRAGSettingsForSession(ctx context.Context, sessionID string, settings session.RAGSettings) error {
	return s.withSessionOperation(ctx, sessionID, func() error {
		return s.dependencies.SessionSettings.SetRAGSettings(ctx, settingscommand.SetRAGSettings{
			SessionID: sessionID,
			Settings:  settings,
		})
	})
}

func (s *ChatService) CurrentSessionPreferences(ctx context.Context) (SessionPreferencesView, error) {
	return s.SessionPreferencesForSession(ctx, s.CurrentSessionID())
}

func (s *ChatService) SessionPreferencesForSession(ctx context.Context, sessionID string) (SessionPreferencesView, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionPreferencesView{}, errors.New("session id is empty")
	}
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return SessionPreferencesView{}, err
	}
	defer unlock()
	current, err := s.dependencies.SessionLoader.Load(ctx, sessionID)
	if err != nil {
		return SessionPreferencesView{}, err
	}
	modelDefaults := generation.Settings{}
	if s.dependencies.ModelMetadata != nil {
		info, ok := s.dependencies.ModelMetadata.GetModelInfo(current.Model)
		if !ok {
			return SessionPreferencesView{}, fmt.Errorf("model metadata not found: %s", current.Model)
		}
		modelDefaults = info.DefaultGenerationSettings
	}
	effective, err := generation.Resolve(modelDefaults, current.GenerationSettings)
	if err != nil {
		return SessionPreferencesView{}, err
	}
	return SessionPreferencesView{
		SessionOverrides: generation.Clone(current.GenerationSettings),
		ModelDefaults:    generation.Clone(modelDefaults),
		Effective:        effective,
		StyleInstruction: current.StyleInstruction,
		RAGSettings:      session.CloneRAGSettings(current.RAGSettings),
	}, nil
}

func (s *ChatService) emitSessionChangedHook(ctx context.Context, sessionID string, reason string) {
	if s.dependencies.Events != nil {
		s.dependencies.Events.SessionChanged(ctx, sessionID, reason)
	}
}

func (s *ChatService) emitSkillReloadedHook(ctx context.Context, skillCount int, reason string) {
	if s.dependencies.Events != nil {
		s.dependencies.Events.SkillReloaded(ctx, skillCount, reason)
	}
}

func (s *ChatService) CompactCurrentSession(ctx context.Context) (ContextInfo, error) {
	return s.CompactSession(ctx, s.CurrentSessionID())
}

func (s *ChatService) CompactSession(ctx context.Context, sessionID string) (ContextInfo, error) {
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return ContextInfo{}, err
	}
	defer unlock()
	return s.dependencies.SessionCompaction.Compact(ctx, compactioncommand.CompactSession{SessionID: sessionID})
}

func (s *ChatService) AddModelConfig(ctx context.Context, command modelcommand.AddConfig) error {
	_, err := s.dependencies.ModelConfig.AddConfig(ctx, command)
	return err
}

func (s *ChatService) CurrentSessionID() string {
	return s.dependencies.CurrentState.State().SessionID
}

func (s *ChatService) CurrentModelID() string {
	return s.dependencies.CurrentState.State().ModelID
}

func (s *ChatService) CurrentPermissionMode() session.PermissionMode {
	return s.dependencies.CurrentState.State().PermissionMode
}

func (s *ChatService) CurrentAgentMode() session.AgentMode {
	return s.dependencies.CurrentState.State().AgentMode
}

func (s *ChatService) CurrentPlan() *agentplan.Plan {
	return s.dependencies.CurrentState.State().Plan
}

func (s *ChatService) CurrentContextWindowK() int {
	return s.dependencies.CurrentState.State().ContextWindowK
}

func (s *ChatService) CurrentUsage() llm.TokenUsage {
	return s.dependencies.CurrentState.State().Usage
}

func (s *ChatService) CurrentLastUsage() llm.TokenUsage {
	return s.dependencies.CurrentState.State().LastUsage
}

func (s *ChatService) CurrentContextInfo() ContextInfo {
	current, err := s.dependencies.CurrentState.CurrentSession()
	if err != nil {
		return ContextInfo{WindowK: contextmgr.DefaultWindowK}
	}

	return s.contextInfo(context.Background(), current)
}

func (s *ChatService) ContextInfoForSession(ctx context.Context, sessionID string) (ContextInfo, error) {
	state, err := s.ContextStateForSession(ctx, sessionID)
	return state.Info, err
}

func (s *ChatService) ContextStateForSession(ctx context.Context, sessionID string) (ContextState, error) {
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return ContextState{Info: ContextInfo{WindowK: contextmgr.DefaultWindowK}}, err
	}
	defer unlock()
	current, err := s.ensureSessionInMemory(ctx, sessionID, false)
	if err != nil {
		return ContextState{Info: ContextInfo{WindowK: contextmgr.DefaultWindowK}}, err
	}
	return ContextState{
		Info:    s.contextInfo(ctx, current),
		Summary: current.Summary,
	}, nil
}

func (s *ChatService) ensureSessionInMemory(ctx context.Context, sessionID string, setCurrent bool) (*session.Session, error) {
	return s.dependencies.SessionLoader.EnsureInMemory(ctx, loadcommand.EnsureInMemory{
		SessionID:  sessionID,
		SetCurrent: setCurrent,
	})
}

func (s *ChatService) withSessionOperation(ctx context.Context, sessionID string, operation func() error) error {
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	return operation()
}

func (s *ChatService) lockSessionOperation(ctx context.Context, sessionID string) (func(), error) {
	s.operationInit.Do(func() { s.operations = newSessionOperationCoordinator() })
	return s.operations.lock(ctx, sessionID)
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
