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
	compaction "myai/core/domain/compaction"
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
	Info       ContextInfo
	Summary    string
	Checkpoint *compaction.Checkpoint
}

type CompactInfo = compactionresult.CompactInfo

type ChatResponse struct {
	SessionID string
	RunID     string
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

	var currentBefore *session.Session
	autoPlan := false
	resumePlan := false
	if s.dependencies.AutoPlanEnabled {
		if s.dependencies.SessionLoader == nil {
			return ChatResponse{}, errors.New("session loader is nil")
		}
		currentBefore, err = s.dependencies.SessionLoader.Load(ctx, sessionID)
		if err != nil {
			return ChatResponse{}, err
		}
		autoPlanDecision := s.classifyAutoPlanRequest(ctx, currentBefore, input)
		autoPlan = autoPlanDecision.ShouldPlan
		resumePlan = shouldResumePlanRequest(currentBefore, input)
	}

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
		SessionID:     sessionID,
		Input:         input,
		ForcePlanMode: autoPlan,
		RAGContext:    retrievalInfo.Prompt,
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
			AppendedMessages:   domainmessage.CloneAll(prepared.AppendedMessages),
			SessionSnapshot:    session.Clone(current),
		})
	}

	if autoPlan {
		return s.generateAndExecutePlan(ctx, current, input, title, retrievalInfo, stream)
	}
	if resumePlan {
		return s.executeExistingPlan(ctx, current.ID, stream, retrievalInfo)
	}
	return s.generateAssistantForSession(ctx, current, input, title, "user request", retrievalInfo, stream, true, false)
}

func (s *ChatService) classifyAutoPlanRequest(ctx context.Context, current *session.Session, input string) AutoPlanDecision {
	classifier := s.dependencies.AutoPlanClassifier
	if classifier == nil {
		classifier = RuleBasedAutoPlanClassifier{}
	}
	decision, err := classifier.Classify(ctx, current, input)
	if err != nil {
		// Classification failure must fail closed. A normal chat turn is safer
		// than unexpectedly entering a workflow that can modify the workspace.
		return AutoPlanDecision{
			Intent: AutoPlanIntentConversation,
			Reason: "auto-plan classification failed: " + err.Error(),
		}
	}
	return decision
}

// generateAndExecutePlan performs the Codex-style two-phase loop inside one
// user request: a read-only planning generation is immediately followed by
// the existing step executor. The planning snapshot is isolated from the
// live session, while its captured plan is persisted to the same session.
func (s *ChatService) generateAndExecutePlan(ctx context.Context, current *session.Session, input string, title string, retrievalInfo chatretrievalresult.Context, stream llm.ChatStreamHandler) (ChatResponse, error) {
	planningSession := session.Clone(current)
	planningSession.AgentMode = session.AgentModePlan
	// The plan is delivered through OnPlanUpdate. Keeping the planning answer
	// out of OnAnswer prevents it from being concatenated with the real step
	// results in the single assistant response shown to the user.
	planningStream := stream
	planningStream.OnAnswer = nil
	planning, err := s.generateAssistantForSession(ctx, planningSession, input, title, "autonomous planning", retrievalInfo, planningStream, true, false)
	if err != nil {
		return planning, err
	}
	if planning.Plan == nil || len(planning.Plan.Steps) == 0 {
		return planning, errors.New("autonomous planning did not produce executable steps")
	}
	if planning.Plan.Status == agentplan.StatusDone {
		return planning, nil
	}
	if s.dependencies.PlanExecution == nil {
		return planning, errors.New("plan execution service is nil")
	}

	execution, err := s.dependencies.PlanExecution.Execute(ctx, plancommand.Execute{
		SessionID: current.ID, ParentRunID: planning.RunID,
		Stream: stream,
	}, nil)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		SessionID: execution.SessionID,
		RunID:     execution.RunID,
		Result:    execution.Result,
		Context:   execution.Context,
		Compact:   execution.Compact,
		Plan:      execution.Plan,
		Retrieval: retrievalInfo,
	}, nil
}

func (s *ChatService) executeExistingPlan(ctx context.Context, sessionID string, stream llm.ChatStreamHandler, retrievalInfo chatretrievalresult.Context) (ChatResponse, error) {
	if s.dependencies.PlanExecution == nil {
		return ChatResponse{}, errors.New("plan execution service is nil")
	}
	execution, err := s.dependencies.PlanExecution.Execute(ctx, plancommand.Execute{
		SessionID: sessionID,
		Stream:    stream,
	}, nil)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		SessionID: execution.SessionID,
		RunID:     execution.RunID,
		Result:    execution.Result,
		Context:   execution.Context,
		Compact:   execution.Compact,
		Plan:      execution.Plan,
		Retrieval: retrievalInfo,
	}, nil
}

func shouldAutoPlanRequest(current *session.Session, input string) bool {
	if current != nil && current.Kind == session.KindSubagent {
		return false
	}
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return false
	}
	// Questions about implementation should receive a normal explanation. Only
	// imperative requests are promoted to the autonomous implementation loop;
	// otherwise phrases such as "为什么要重构" would unexpectedly edit files.
	if isImplementationQuestion(input) {
		return false
	}
	action := false
	for _, keyword := range []string{"实现", "开发", "添加", "新增", "加上", "增加", "修改", "改造", "重构", "重写", "修复", "完善", "优化", "升级", "补充", "接入", "支持", "集成", "构建", "编写", "配置", "部署", "删除", "替换", "迁移", "做一个", "写一个", "implement", "develop", "build", "add", "create", "delete", "remove", "fix", "refactor", "rewrite", "improve", "upgrade", "migrate", "feature"} {
		if strings.Contains(input, keyword) {
			action = true
			break
		}
	}
	if !action {
		return false
	}
	for _, contextKeyword := range []string{"代码", "代码库", "仓库", "项目", "功能", "需求", "组件", "接口", "后端", "前端", "安卓", "android", "api", "数据库", "文件", "git", "app", "应用", "工程", "服务", "页面", "界面", "脚本", "模块", "任务", "流程", "逻辑", "更新", "版本", "登录", "认证", "上传", "下载", "通知", "动画", "长连接", "websocket", "ws", "ota", "子智能体", "agent", "test", "测试", "编译", "部署", "repository", "service", "frontend", "backend", "typescript", "javascript", "golang", "kotlin", "java", "swift", "css", "html", "sql", "bug", "错误", "问题", "消息"} {
		if strings.Contains(input, contextKeyword) {
			return true
		}
	}
	return hasImplementationContext(current)
}

func isImplementationQuestion(input string) bool {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return false
	}
	question := strings.HasSuffix(input, "吗") || strings.HasSuffix(input, "？") || strings.HasSuffix(input, "?")
	for _, marker := range []string{"为什么", "为何", "如何", "怎么", "能否", "是否", "可不可以", "可以吗", "能不能", "why ", "how ", "can ", "could ", "should ", "is it "} {
		if strings.HasPrefix(input, marker) || strings.Contains(input, marker) {
			question = true
			break
		}
	}
	if !question {
		return false
	}
	// Explicit imperative cues keep requests such as "能不能帮我修复" and
	// "请直接实现" in the implementation path, even when punctuated as a
	// question. "请问/请告诉" remain explanatory questions.
	for _, cue := range []string{"帮我", "请你", "请直接", "直接", "开始", "现在", "落地", "执行"} {
		if strings.Contains(input, cue) {
			return false
		}
	}
	if strings.HasPrefix(input, "请") && !strings.HasPrefix(input, "请问") && !strings.HasPrefix(input, "请告诉") && !strings.HasPrefix(input, "请解释") {
		return false
	}
	return true
}

func hasImplementationContext(current *session.Session) bool {
	if current == nil {
		return false
	}
	const maxRecentUserMessages = 8
	seen := 0
	for index := len(current.Messages) - 1; index >= 0 && seen < maxRecentUserMessages; index-- {
		message := current.Messages[index]
		if message.Role != domainmessage.RoleUser || message.IsSynthetic() {
			continue
		}
		seen++
		text := strings.ToLower(message.Text())
		for _, keyword := range []string{"代码", "代码库", "仓库", "项目", "功能", "组件", "接口", "后端", "前端", "安卓", "android", "api", "数据库", "文件", "git", "app", "应用", "工程", "服务", "页面", "脚本", "模块", "websocket", "ota", "agent", "test", "测试", "编译", "部署", "repository", "service", "typescript", "javascript", "golang", "kotlin", "java", "swift", "css", "html", "sql", "bug"} {
			if strings.Contains(text, keyword) {
				return true
			}
		}
	}
	return false
}

func shouldResumePlanRequest(current *session.Session, input string) bool {
	if current == nil || current.Kind == session.KindSubagent || current.CurrentPlan == nil {
		return false
	}
	if !agentplan.IsExecutableStatus(current.CurrentPlan.Status) || current.CurrentPlan.Status == agentplan.StatusDone {
		return false
	}
	switch strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(input)), " ")) {
	case "继续", "继续执行", "恢复计划", "执行计划", "开始执行", "continue", "resume", "resume plan", "execute plan", "run plan":
		return true
	default:
		return false
	}
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
			SyntheticReason: syntheticReason, AppendedMessages: domainmessage.CloneAll(prepared.AppendedMessages), SessionSnapshot: session.Clone(current),
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
		Internal:      reason == "autonomous planning" || reason == "recover plan",
	})
	if err != nil {
		return ChatResponse{}, err
	}

	return ChatResponse{
		SessionID: response.SessionID,
		RunID:     response.RunID,
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

func (s *ChatService) UpdateModelConfig(ctx context.Context, command modelcommand.UpdateConfig) error {
	_, err := s.dependencies.ModelConfig.UpdateConfig(ctx, command)
	return err
}

func (s *ChatService) SetModelEnabled(ctx context.Context, command modelcommand.SetEnabled) error {
	_, err := s.dependencies.ModelConfig.SetEnabled(ctx, command)
	return err
}

func (s *ChatService) SetDefaultModel(ctx context.Context, command modelcommand.SetDefault) error {
	_, err := s.dependencies.ModelConfig.SetDefault(ctx, command)
	return err
}

func (s *ChatService) DeleteModelConfig(ctx context.Context, command modelcommand.DeleteConfig) error {
	_, err := s.dependencies.ModelConfig.DeleteConfig(ctx, command)
	return err
}

func (s *ChatService) TestModelConfig(ctx context.Context, command modelcommand.AddConfig) (int64, error) {
	result, err := s.dependencies.ModelConfig.TestConfig(ctx, command)
	if err != nil {
		return 0, err
	}
	return result.LatencyMS, nil
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
	info := s.contextInfo(ctx, current)
	// 以已经完成校验的 Snapshot 为准，不能直接返回 Session 中可能失效的旧指针。
	checkpoint := info.Checkpoint
	summary := current.Summary
	if checkpoint != nil {
		summary = checkpoint.DisplaySummary()
	}
	return ContextState{Info: info, Summary: summary, Checkpoint: checkpoint}, nil
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
