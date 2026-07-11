package service

import (
	"context"

	compactionapi "myai/core/application/chat/compaction/api"
	chatcontextapi "myai/core/application/chat/context/api"
	generationapi "myai/core/application/chat/generation/api"
	planapi "myai/core/application/chat/plan/api"
	planport "myai/core/application/chat/plan/port"
	modelapi "myai/core/application/model/api"
	bootstrapapi "myai/core/application/session/bootstrap/api"
	currentapi "myai/core/application/session/current/api"
	lifecycleapi "myai/core/application/session/lifecycle/api"
	loadapi "myai/core/application/session/load/api"
	messageapi "myai/core/application/session/message/api"
	queryapi "myai/core/application/session/query/api"
	settingsapi "myai/core/application/session/settings/api"
	skillapi "myai/core/application/skill/api"
	modelport "myai/core/port/model"
)

type ChatEventPublisher interface {
	SessionChanged(ctx context.Context, sessionID string, reason string)
	SkillReloaded(ctx context.Context, skillCount int, reason string)
}

type ChatDependencies struct {
	// ChatDependencies 只组合应用接口和共享 port；具体 adapter 在 composition/chat 中注入。
	Models modelport.Registry

	GenerationTasks   generationapi.TaskService
	PlanExecution     planapi.Service
	SessionCompaction compactionapi.SessionService
	ContextQueries    chatcontextapi.QueryService
	UserMessages      planport.UserMessagePersistence

	SessionLoader    loadapi.Service
	SessionLifecycle lifecycleapi.UseCase
	SessionSettings  settingsapi.UseCase
	SessionQueries   queryapi.SessionQueryService
	MessageQueries   queryapi.MessageQueryService
	MessageCommands  messageapi.CommandService
	CurrentState     currentapi.StateQueryService
	SessionBootstrap bootstrapapi.Service

	ModelConfig  modelapi.ConfigService
	ModelQueries modelapi.QueryService
	SkillCatalog skillapi.CatalogService
	Events       ChatEventPublisher
}
