package chat

import (
	"context"
	"log"
	"time"

	generationadapter "myai/core/adapter/chat/generation"
	pendinginput "myai/core/adapter/chat/pendinginput"
	taskrecorder "myai/core/adapter/history/taskrecorder"
	hookevents "myai/core/adapter/hook/events"
	hooklifecycle "myai/core/adapter/hook/lifecycle"
	uuidadapter "myai/core/adapter/id/uuid"
	modelusage "myai/core/adapter/modelusage/session"
	chatmessagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	chatmessagerepository "myai/core/adapter/persistence/chatmessage/repository"
	toolrecordsrepository "myai/core/adapter/persistence/toolrecords/repository"
	sessionplanstate "myai/core/adapter/plan/sessionstate"
	memorysession "myai/core/adapter/session/memory"
	toolcatalog "myai/core/adapter/tool/catalog"
	toolexecutor "myai/core/adapter/tool/executor"
	agentrunapi "myai/core/application/agentrun/api"
	agentrunservice "myai/core/application/agentrun/service"
	compactionservice "myai/core/application/chat/compaction/service"
	chatcontextservice "myai/core/application/chat/context/service"
	generationcommand "myai/core/application/chat/generation/command"
	generationservice "myai/core/application/chat/generation/service"
	planservice "myai/core/application/chat/plan/service"
	chatretrievalapi "myai/core/application/chat/retrieval/api"
	chatretrievalservice "myai/core/application/chat/retrieval/service"
	searchapi "myai/core/application/knowledge/search/api"
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
	domainmessage "myai/core/domain/message"
	"myai/core/hook"
	agentrunport "myai/core/port/agentrun"
	asyncport "myai/core/port/async"
	cacheport "myai/core/port/cache"
	modelport "myai/core/port/model"
	persistenceport "myai/core/port/persistence"
	repository "myai/core/port/repository"
	"myai/core/service"
	"myai/core/session"
	"myai/core/skill"
	"myai/core/tool"
)

const (
	defaultUserID     = "local"
	currentSessionTTL = 24 * time.Hour
)

type ModelRegistry interface {
	modelport.MutableRegistry
	modelport.MetadataProvider
}

type Configuration struct {
	// Configuration 只接收进程已经创建好的基础设施，BuildDependencies 再把它们装配成应用服务。
	Models           ModelRegistry
	ModelFactory     modelport.Factory
	Sessions         *memorysession.Store
	Store            persistenceport.Store
	Cache            cacheport.CurrentSessionCache
	Async            asyncport.Executor
	Tools            *tool.RegisterTools
	Skills           *skill.Manager
	Hooks            *hook.Manager
	DefaultModel     string
	UserID           string
	KnowledgeSearch  searchapi.Service
	AgentRuns        agentrunport.Repository
	AgentRunObserver agentrunport.CompletionObserver
	IntentController *service.IntentController
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
	var runCommands agentrunapi.CommandService
	var runQueries agentrunapi.QueryService
	if configuration.AgentRuns != nil {
		baseCommands := agentrunservice.CommandService{Repository: configuration.AgentRuns, IDs: uuidadapter.Generator{}}
		runCommands = baseCommands
		if configuration.AgentRunObserver != nil {
			runCommands = agentrunservice.ObservingCommandService{Inner: baseCommands, Observer: configuration.AgentRunObserver}
		}
		runQueries = agentrunservice.QueryService{Repository: configuration.AgentRuns}
	}

	// 第一组：会话加载、生命周期、查询和设置用例。
	loader := loadservice.LoadService{
		Memory:   configuration.Sessions,
		Sessions: configuration.Store,
		Messages: configuration.Store,
	}
	lifecycle := lifecycleservice.LifecycleService{
		Memory:   configuration.Sessions,
		Loader:   loader,
		Sessions: configuration.Store,
		Messages: configuration.Store,
	}
	settings := settingsservice.SettingsService{
		Memory: configuration.Sessions,
		Loader: loader,
		Models: configuration.Models,
	}
	sessionQueries := queryservice.SessionQueryService{
		Sessions: configuration.Store,
		Assets:   configuration.Store,
	}
	messageQueries := queryservice.MessageQueryService{
		Store:         configuration.Store,
		Memory:        configuration.Sessions,
		MemoryRecords: chatmessagemapper.Mapper{IDs: uuidadapter.Generator{}},
	}
	runtimePrompts := runtimeservice.NewSessionPromptProvider(configuration.Skills)
	messageCommands := messageservice.CommandService{
		Loader:              loader,
		Memory:              configuration.Sessions,
		RuntimeInstructions: runtimePrompts,
	}
	currentState := currentservice.StateQueryService{
		Memory:       configuration.Sessions,
		DefaultModel: configuration.DefaultModel,
	}
	sessionPersistence := persistenceservice.PersistenceService{
		Sessions:     configuration.Store,
		Memory:       configuration.Sessions,
		DefaultModel: configuration.DefaultModel,
	}

	currentSession := currentservice.SessionService{
		Cache:  configuration.Cache,
		UserID: userID,
		TTL:    currentSessionTTL,
	}
	// 第二组：把应用层消息写入请求适配为具体仓库写入，并通过线程池异步落库。
	messageWriter := chatmessagerepository.Writer{
		Messages:    configuration.Store,
		Sessions:    sessionPersistence,
		Transcripts: configuration.Store,
		IDs:         uuidadapter.Generator{},
	}
	asyncTasks := runtimeservice.AsyncTaskService{Executor: configuration.Async}
	sessionQueue := generationadapter.NewSessionQueue(asyncTasks)
	sessionQueue.OnPanic = func(err error) {
		log.Print(err)
	}
	messageCommands.Regeneration = generationadapter.RegenerationPersistence{
		Messages: messageWriter,
		Queue:    sessionQueue,
		OnError: func(err error) {
			log.Print(err)
		},
	}
	userMessages := generationadapter.UserMessagePersistence{
		Messages: messageWriter,
		Queue:    sessionQueue,
		Async:    asyncTasks,
		OnError: func(err error) {
			log.Print(err)
		},
	}

	// 第三组：生成链路。固定系统提示词保留在 Session，Plan/Skill 作为每轮运行时指令注入。
	contexts := chatcontextservice.SnapshotService{}
	var retrievalContext chatretrievalapi.ContextPreparer
	if configuration.KnowledgeSearch != nil {
		retrievalContext = chatretrievalservice.ContextService{
			Search:    configuration.KnowledgeSearch,
			Policy:    chatretrievalservice.DefaultTriggerPolicy{},
			Formatter: chatretrievalservice.ContextFormatter{},
		}
	}
	contextQueries := chatcontextservice.QueryService{
		Contexts: contexts,
	}
	summaryStore := generationadapter.SummaryStore{
		Sessions: sessionPersistence,
		Memory:   configuration.Sessions,
	}
	responseCommit := generationservice.ResponseCommitService{
		Memory:       configuration.Sessions,
		PlanCapturer: planserviceapp.CaptureService{},
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
	turnHooks := hooklifecycle.Bridge{Hooks: configuration.Hooks}
	turnInputQueue := pendinginput.NewQueue()
	agentLoop := generationservice.AgentLoopService{
		Contexts:     contexts,
		Tools:        toolcatalog.Catalog{Tools: configuration.Tools},
		ToolExecutor: toolExecutor,
		Compactor:    compactor,
		TurnHooks:    turnHooks,
		PendingInput: turnInputQueue,
		OnPendingInputAppended: func(current *session.Session, messages []domainmessage.Message) {
			// 1. AgentLoop 领取 mailbox 后先把新增消息写入持久化队列，避免只留在内存。
			if current == nil || len(messages) == 0 {
				return
			}
			// 2. 运行中的 turn 也可能收到子代理消息，按会话顺序写入增量消息。
			userMessages.PersistUserMessage(generationcommand.PersistUserMessage{
				SessionID: current.ID, Model: current.Model, Input: messages[len(messages)-1].Text(),
				AppendedMessages: domainmessage.CloneAll(messages), SessionSnapshot: session.Clone(current),
			})
		},
		OnCompactError: func(err error) {
			log.Printf("auto compact failed: %v", err)
		},
		ToolRecords: toolrecordsrepository.Recorder{
			Persistence:        configuration.Store,
			IDs:                uuidadapter.Generator{},
			RunAsync:           asyncTasks.Submit,
			RunAsyncForSession: sessionQueue.Submit,
			OnError: func(err error) {
				log.Printf("save tool execution records failed: %v", err)
			},
		},
	}
	generationPersistence := generationadapter.Persistence{
		Messages: messageWriter,
		Queue:    sessionQueue,
		Async:    asyncTasks,
		OnError: func(err error) {
			log.Print(err)
		},
	}
	assistantGeneration := generationservice.AssistantGenerationService{
		Models:            configuration.Models,
		ModelMetadata:     configuration.Models,
		Contexts:          contexts,
		Compactor:         compactor,
		AgentRunner:       agentLoop,
		ResponseCommitter: responseCommit,
		Persistence:       generationPersistence,
		OnCompactError: func(err error) {
			log.Printf("auto compact failed: %v", err)
		},
	}
	generationTasks := generationservice.TaskService{
		RequestIDs: uuidadapter.Generator{},
		Recorders:  taskrecorder.Factory{},
		Generator:  assistantGeneration,
		Runs:       runCommands,
		TurnHooks:  turnHooks,
		OnSaveError: func(err error) {
			log.Printf("save task history checkpoint failed: %v", err)
		},
		OnCloseError: func(err error) {
			log.Printf("close task history recorder failed: %v", err)
		},
		OnRunError: func(err error) {
			log.Printf("record agent run failed: %v", err)
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
		Models:           configuration.Models,
		Sessions:         loader,
		Messages:         messageCommands,
		Generation:       generationTasks,
		PlanStates:       planStates,
		UserMessages:     userMessages,
		Events:           events,
		Recovery:         planservice.GenerationRecoveryPlanner{Messages: messageCommands, Generation: generationTasks},
		MaxReplans:       1,
		MaxParallelSteps: 3,
		Runs:             runCommands,
		OnRunError: func(err error) {
			log.Printf("record plan run failed: %v", err)
		},
	}

	// ChatService 只拿接口，不知道 Mongo、Redis、LangChainGo 等具体技术实现。
	classifier := service.AutoPlanClassifier(service.ModelAutoPlanClassifier{Models: configuration.Models, Metadata: configuration.Models})
	if configuration.IntentController != nil {
		classifier = configuration.IntentController
	}
	continuationControl, _ := configuration.Store.(repository.SessionContinuationStore)
	return service.ChatDependencies{
		ContinuationControl: continuationControl,
		Models:              configuration.Models,
		AutoPlanEnabled:     true,
		AutoPlanClassifier:  classifier,
		IntentController:    configuration.IntentController,
		ModelMetadata:       configuration.Models,

		GenerationTasks: generationTasks,
		TurnInputQueue:  turnInputQueue,
		PlanExecution:   planExecution,
		SessionCompaction: compactionservice.SessionService{
			Sessions:  loader,
			Models:    configuration.Models,
			Compactor: compactor,
			Contexts:  contextQueries,
		},
		ContextQueries:   contextQueries,
		RetrievalContext: retrievalContext,
		UserMessages:     userMessages,

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
			Usage:      modelusage.Checker{Sessions: configuration.Store},
			Default:    configuration.Sessions,
		},
		ModelQueries:    modelservice.QueryService{Catalog: configuration.Models},
		SkillCatalog:    skillCatalog,
		Events:          events,
		AgentRunQueries: runQueries,
		OnPendingTurnError: func(err error) {
			log.Printf("background parent turn failed: %v", err)
		},
	}
}
