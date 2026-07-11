package chat

import (
	"context"
	"log"
	"time"

	generationadapter "myai/core/adapter/chat/generation"
	taskrecorder "myai/core/adapter/history/taskrecorder"
	hookevents "myai/core/adapter/hook/events"
	uuidadapter "myai/core/adapter/id/uuid"
	chatmessagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	chatmessagerepository "myai/core/adapter/persistence/chatmessage/repository"
	toolrecordsrepository "myai/core/adapter/persistence/toolrecords/repository"
	sessionplanstate "myai/core/adapter/plan/sessionstate"
	memorysession "myai/core/adapter/session/memory"
	toolcatalog "myai/core/adapter/tool/catalog"
	toolexecutor "myai/core/adapter/tool/executor"
	compactionservice "myai/core/application/chat/compaction/service"
	chatcontextservice "myai/core/application/chat/context/service"
	generationservice "myai/core/application/chat/generation/service"
	planservice "myai/core/application/chat/plan/service"
	modelservice "myai/core/application/model/service"
	planserviceapp "myai/core/application/plan/service"
	runtimeservice "myai/core/application/runtime/service"
	bootstrapservice "myai/core/application/session/bootstrap/service"
	sessioncommand "myai/core/application/session/command"
	currentservice "myai/core/application/session/current/service"
	lifecycleservice "myai/core/application/session/lifecycle/service"
	loadservice "myai/core/application/session/load/service"
	messageservice "myai/core/application/session/message/service"
	persistenceservice "myai/core/application/session/persistence/service"
	queryservice "myai/core/application/session/query/service"
	settingsservice "myai/core/application/session/settings/service"
	skillservice "myai/core/application/skill/service"
	"myai/core/hook"
	asyncport "myai/core/port/async"
	cacheport "myai/core/port/cache"
	modelport "myai/core/port/model"
	persistenceport "myai/core/port/persistence"
	"myai/core/service"
	"myai/core/skill"
	"myai/core/tool"
)

const (
	defaultUserID     = "local"
	currentSessionTTL = 24 * time.Hour
)

type Configuration struct {
	// Configuration 只接收进程已经创建好的基础设施，BuildDependencies 再把它们装配成应用服务。
	Models       modelport.MutableRegistry
	ModelFactory modelport.Factory
	Sessions     *memorysession.Store
	Store        persistenceport.Store
	Cache        cacheport.CurrentSessionCache
	Async        asyncport.Executor
	Tools        *tool.RegisterTools
	Skills       *skill.Manager
	Hooks        *hook.Manager
	DefaultModel string
	UserID       string
}

func NewService(configuration Configuration) *service.ChatService {
	return service.NewChatService(BuildDependencies(configuration))
}

func BuildDependencies(configuration Configuration) service.ChatDependencies {
	// 本函数是聊天模块唯一的 composition root。阅读“接口最终由谁实现”时，应从这里开始。
	userID := configuration.UserID
	if userID == "" {
		userID = defaultUserID
	}

	// 第一组：会话加载、生命周期、查询和设置用例。
	loader := loadservice.LoadService{
		Sessions: configuration.Store,
		Messages: configuration.Store,
	}
	lifecycle := lifecycleservice.LifecycleService{
		Loader:   loader,
		Sessions: configuration.Store,
		Messages: configuration.Store,
	}
	settings := settingsservice.SettingsService{
		Loader: loader,
		Models: configuration.Models,
	}
	sessionQueries := queryservice.SessionQueryService{
		Sessions: configuration.Store,
		Assets:   configuration.Store,
	}
	messageQueries := queryservice.MessageQueryService{
		Store:         configuration.Store,
		MemoryRecords: chatmessagemapper.Mapper{IDs: uuidadapter.Generator{}},
	}
	messageCommands := messageservice.CommandService{}
	currentState := currentservice.StateQueryService{DefaultModel: configuration.DefaultModel}
	sessionPersistence := persistenceservice.PersistenceService{
		Sessions:     configuration.Store,
		DefaultModel: configuration.DefaultModel,
	}
	if configuration.Sessions != nil {
		loader.Memory = configuration.Sessions
		lifecycle.Memory = configuration.Sessions
		settings.Memory = configuration.Sessions
		messageQueries.Memory = configuration.Sessions
		messageCommands.Loader = loader
		messageCommands.Memory = configuration.Sessions
		currentState.Memory = configuration.Sessions
		sessionPersistence.Memory = configuration.Sessions
	}

	currentSession := currentservice.SessionService{
		Cache:  configuration.Cache,
		UserID: userID,
		TTL:    currentSessionTTL,
	}
	// 第二组：把应用层消息写入请求适配为具体仓库写入，并通过线程池异步落库。
	messageWriter := chatmessagerepository.Writer{
		Messages: configuration.Store,
		Sessions: sessionPersistence,
		IDs:      uuidadapter.Generator{},
	}
	asyncTasks := runtimeservice.AsyncTaskService{Executor: configuration.Async}
	userMessages := generationadapter.UserMessagePersistence{
		Messages: messageWriter,
		Async:    asyncTasks,
		OnError: func(err error) {
			log.Print(err)
		},
	}

	// 第三组：生成链路。固定系统提示词保留在 Session，Plan/Skill 作为每轮运行时指令注入。
	runtimePrompts := runtimeservice.NewSessionPromptProvider(configuration.Skills)
	contexts := chatcontextservice.SnapshotService{}
	contextQueries := chatcontextservice.QueryService{
		Contexts:            contexts,
		RuntimeInstructions: runtimePrompts,
	}
	summaryStore := generationadapter.SummaryStore{
		Sessions: sessionPersistence,
	}
	responseCommit := generationservice.ResponseCommitService{PlanCapturer: planserviceapp.CaptureService{}}
	if configuration.Sessions != nil {
		summaryStore.Memory = configuration.Sessions
		responseCommit.Memory = configuration.Sessions
	}
	compactor := compactionservice.CompactService{
		Contexts:   contexts,
		Summarizer: compactionservice.SummaryService{},
		Summaries:  summaryStore,
	}

	// 工具执行器统一处理本地工具和 MCP 工具，并在同一位置接入 Hook 与执行记录。
	toolExecutor := toolexecutor.Executor{
		Registry: configuration.Tools,
		Hooks: toolexecutor.HookBridge{
			Hooks: configuration.Hooks,
			OnPostError: func(err error) {
				log.Printf("post tool hook failed: %v", err)
			},
		},
	}
	agentLoop := generationservice.AgentLoopService{
		Contexts:            contexts,
		Tools:               toolcatalog.Catalog{Tools: configuration.Tools},
		RuntimeInstructions: runtimePrompts,
		ToolExecutor:        toolExecutor,
		ToolRecords: toolrecordsrepository.Recorder{
			Persistence: configuration.Store,
			IDs:         uuidadapter.Generator{},
			RunAsync:    asyncTasks.Submit,
			OnError: func(err error) {
				log.Printf("save tool execution records failed: %v", err)
			},
		},
	}
	generationPersistence := generationadapter.Persistence{
		Messages:       messageWriter,
		CurrentSession: currentSession,
		Async:          asyncTasks,
		OnError: func(err error) {
			log.Print(err)
		},
	}
	assistantGeneration := generationservice.AssistantGenerationService{
		Models:              configuration.Models,
		RuntimeInstructions: runtimePrompts,
		Contexts:            contexts,
		Compactor:           compactor,
		AgentRunner:         agentLoop,
		ResponseCommitter:   responseCommit,
		Persistence:         generationPersistence,
		OnCompactError: func(err error) {
			log.Printf("auto compact failed: %v", err)
		},
	}
	generationTasks := generationservice.TaskService{
		RequestIDs: uuidadapter.Generator{},
		Recorders:  taskrecorder.Factory{},
		Generator:  assistantGeneration,
		OnSaveError: func(err error) {
			log.Printf("save task history checkpoint failed: %v", err)
		},
		OnCloseError: func(err error) {
			log.Printf("close task history recorder failed: %v", err)
		},
	}

	// 第四组：Plan 状态先更新内存 Session，再通过回调持久化整个会话。
	planRepository := sessionplanstate.NewRepository(configuration.Sessions, func(ctx context.Context, sessionID string, model string) error {
		return sessionPersistence.Save(ctx, sessioncommand.SaveSession{
			SessionID: sessionID,
			Model:     model,
		})
	})
	planStates := planserviceapp.StatePersistenceService{Repository: planRepository}

	bootstrap := bootstrapservice.BootstrapService{
		Cache:       currentSession,
		Persistence: sessionPersistence,
	}
	if configuration.Sessions != nil {
		bootstrap.Lifecycle = lifecycle
		bootstrap.State = currentState
	}

	skillCatalog := skillservice.CatalogService{}
	if configuration.Skills != nil {
		skillCatalog.Catalog = configuration.Skills
	}
	events := hookevents.Publisher{
		OnError: func(err error) {
			log.Print(err)
		},
	}
	if configuration.Hooks != nil {
		events.Hooks = configuration.Hooks
	}
	lifecycleUseCase := lifecycleservice.UseCase{
		Lifecycle:    lifecycle,
		Persistence:  sessionPersistence,
		Current:      currentSession,
		SessionQuery: sessionQueries,
		Events:       events,
	}
	settingsUseCase := settingsservice.UseCase{
		Settings:    settings,
		Persistence: sessionPersistence,
		Events:      events,
	}
	planExecution := planservice.ExecutionService{
		Models:       configuration.Models,
		Sessions:     loader,
		Messages:     messageCommands,
		Generation:   generationTasks,
		PlanStates:   planStates,
		UserMessages: userMessages,
		Events:       events,
	}

	// ChatService 只拿接口，不知道 Mongo、Redis、LangChainGo 等具体技术实现。
	return service.ChatDependencies{
		Models: configuration.Models,

		GenerationTasks: generationTasks,
		PlanExecution:   planExecution,
		SessionCompaction: compactionservice.SessionService{
			Sessions:  loader,
			Models:    configuration.Models,
			Compactor: compactor,
			Contexts:  contextQueries,
		},
		ContextQueries: contextQueries,
		UserMessages:   userMessages,

		SessionLoader:    loader,
		SessionLifecycle: lifecycleUseCase,
		SessionSettings:  settingsUseCase,
		SessionQueries:   sessionQueries,
		MessageQueries:   messageQueries,
		MessageCommands:  messageCommands,
		CurrentState:     currentState,
		SessionBootstrap: bootstrap,

		ModelConfig: modelservice.ConfigService{
			Repository: configuration.Store,
			Registry:   configuration.Models,
			Factory:    configuration.ModelFactory,
		},
		ModelQueries: modelservice.QueryService{Catalog: configuration.Models},
		SkillCatalog: skillCatalog,
		Events:       events,
	}
}
