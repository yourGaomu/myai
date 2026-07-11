package agent

import (
	"context"

	sessionresult "myai/core/application/session/result"
	"myai/core/llm"
	agentplan "myai/core/plan"
	"myai/core/service"
	"myai/core/session"
	"myai/core/skill"
)

type ChatGenerationFacade interface {
	// 远程层只声明实际使用的方法，避免依赖 ChatService 的全部实现细节。
	SendMessageStreamForSession(ctx context.Context, sessionID string, input string, stream llm.ChatStreamHandler) (service.ChatResponse, error)
	RegenerateLastMessageStreamForSession(ctx context.Context, sessionID string, stream llm.ChatStreamHandler) (service.ChatResponse, error)
	ExecutePlanStreamForSession(ctx context.Context, sessionID string, stream llm.ChatStreamHandler, onPlanUpdate func(*agentplan.Plan)) (service.ChatResponse, error)
}

type SessionLifecycleFacade interface {
	NewSession(ctx context.Context) error
	LoadSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	RestoreSession(ctx context.Context, sessionID string) error
}

type SessionQueryFacade interface {
	ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]sessionresult.SessionListItem, error)
	ListSessionMessages(ctx context.Context, sessionID string) ([]sessionresult.MessageListItem, error)
	SessionHistoryMeta(ctx context.Context, sessionID string) (sessionresult.MessageHistoryMeta, error)
	ListSessionMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]sessionresult.MessageListItem, bool, error)
	ListAssets(ctx context.Context, sessionID string, limit int) ([]sessionresult.AssetListItem, error)
	ContextInfoForSession(ctx context.Context, sessionID string) (service.ContextInfo, error)
}

type SessionSettingsFacade interface {
	SetPermissionModeForSession(ctx context.Context, sessionID string, mode string) error
	SetAgentModeForSession(ctx context.Context, sessionID string, mode string) error
	SetContextWindowKForSession(ctx context.Context, sessionID string, windowK int) error
	CompactSession(ctx context.Context, sessionID string) (service.ContextInfo, error)
	SwitchModelForSession(ctx context.Context, sessionID string, modelID string) error
}

type CurrentSessionFacade interface {
	CurrentSessionID() string
	CurrentModelID() string
	CurrentPermissionMode() session.PermissionMode
	CurrentAgentMode() session.AgentMode
	CurrentPlan() *agentplan.Plan
	CurrentContextWindowK() int
	CurrentUsage() llm.TokenUsage
	CurrentLastUsage() llm.TokenUsage
}

type CatalogFacade interface {
	ListModels() []llm.ModelInfo
	ListSkills(ctx context.Context) ([]skill.Skill, error)
	ReloadSkills(ctx context.Context, reason string) ([]skill.Skill, error)
	SkillRoot() string
}

type ChatFacade interface {
	ChatGenerationFacade
	SessionLifecycleFacade
	SessionQueryFacade
	SessionSettingsFacade
	CurrentSessionFacade
	CatalogFacade
}
