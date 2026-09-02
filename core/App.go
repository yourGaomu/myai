package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	adapterthreadpool "myai/core/adapter/async/threadpool"
	adapterredis "myai/core/adapter/cache/redis"
	grpcprocessor "myai/core/adapter/documentprocessor/grpc"
	assetsource "myai/core/adapter/documentsource/asset"
	embeddingmemory "myai/core/adapter/embedding/memory"
	openaiembedding "myai/core/adapter/embedding/openaicompatible"
	localexecutor "myai/core/adapter/execution/local"
	contenthashid "myai/core/adapter/id/contenthash"
	snowflakeid "myai/core/adapter/id/snowflake"
	uuidadapter "myai/core/adapter/id/uuid"
	sqlitefts5 "myai/core/adapter/keywordstore/sqlitefts5"
	memorydreamadapter "myai/core/adapter/memory/dream"
	memoryextractor "myai/core/adapter/memory/extractor"
	adaptermodel "myai/core/adapter/model/langchaingo"
	minioadapter "myai/core/adapter/objectstorage/minio"
	agentrunmemory "myai/core/adapter/persistence/memory/agentrun"
	aimemory "myai/core/adapter/persistence/memory/memory"
	agentrunmongo "myai/core/adapter/persistence/mongo/agentrun/repository"
	knowledgemongo "myai/core/adapter/persistence/mongo/knowledge/repository"
	memorymongo "myai/core/adapter/persistence/mongo/memory/repository"
	adaptermongo "myai/core/adapter/persistence/mongo/repository"
	subagentmongo "myai/core/adapter/persistence/mongo/subagent/repository"
	sqlitehistory "myai/core/adapter/persistence/sqlite/history/repository"
	opensandboxadapter "myai/core/adapter/sandbox/opensandbox"
	memorysession "myai/core/adapter/session/memory"
	subagentchat "myai/core/adapter/subagent/chat"
	subagentevents "myai/core/adapter/subagent/events"
	subagentlocal "myai/core/adapter/subagent/local"
	subagentmemory "myai/core/adapter/subagent/memory"
	subagentsession "myai/core/adapter/subagent/session"
	milvusadapter "myai/core/adapter/vectorstore/milvus"
	sqlitevec "myai/core/adapter/vectorstore/sqlitevec"
	opensandboxworkspace "myai/core/adapter/workspace/opensandbox"
	workspaceRouter "myai/core/adapter/workspace/router"
	snapshotworkspace "myai/core/adapter/workspace/snapshot"
	agentrunservice "myai/core/application/agentrun/service"
	catalogapi "myai/core/application/knowledge/catalog/api"
	catalogservice "myai/core/application/knowledge/catalog/service"
	documentapi "myai/core/application/knowledge/document/api"
	documentservice "myai/core/application/knowledge/document/service"
	embeddingservice "myai/core/application/knowledge/embedding/service"
	indexingapi "myai/core/application/knowledge/indexing/api"
	indexingservice "myai/core/application/knowledge/indexing/service"
	queryapi "myai/core/application/knowledge/query/api"
	queryservice "myai/core/application/knowledge/query/service"
	retrievalapi "myai/core/application/knowledge/retrieval/api"
	retrievalservice "myai/core/application/knowledge/retrieval/service"
	searchapi "myai/core/application/knowledge/search/api"
	searchservice "myai/core/application/knowledge/search/service"
	memorycatalogapi "myai/core/application/memory/catalog/api"
	memorycatalogservice "myai/core/application/memory/catalog/service"
	memorydreamapi "myai/core/application/memory/dream/api"
	memorydreamservice "myai/core/application/memory/dream/service"
	memoryextractionapi "myai/core/application/memory/extraction/api"
	memoryextractioncommand "myai/core/application/memory/extraction/command"
	memoryextractionservice "myai/core/application/memory/extraction/service"
	memoryretrievalapi "myai/core/application/memory/retrieval/api"
	memoryretrievalservice "myai/core/application/memory/retrieval/service"
	modelcommand "myai/core/application/model/command"
	modelservice "myai/core/application/model/service"
	sessionpersistenceservice "myai/core/application/session/persistence/service"
	subagentapi "myai/core/application/subagent/api"
	subagentcommand "myai/core/application/subagent/command"
	subagentservice "myai/core/application/subagent/service"
	"myai/core/asset"
	chatcomposition "myai/core/composition/chat"
	appconfig "myai/core/config"
	domainmemory "myai/core/domain/memory"
	"myai/core/hook"
	"myai/core/infra"
	"myai/core/llm"
	"myai/core/mcp"
	pluginruntime "myai/core/plugin"
	agentrunport "myai/core/port/agentrun"
	cacheport "myai/core/port/cache"
	executionport "myai/core/port/execution"
	knowledgeport "myai/core/port/knowledge"
	documentprocessorport "myai/core/port/knowledge/documentprocessor"
	memoryport "myai/core/port/memory"
	persistenceport "myai/core/port/persistence"
	sandboxport "myai/core/port/sandbox"
	subagentport "myai/core/port/subagent"
	workspaceport "myai/core/port/workspace"
	"myai/core/service"
	"myai/core/skill"
	"myai/core/tool"
	"myai/core/tool/local"
	tooldef "myai/core/tool/tool"
)

type Application struct {
	// Application 是进程级资源容器，作用类似 Spring Boot 的 ApplicationContext。
	// 它只负责创建和持有基础设施，不承载聊天、Plan 等业务规则。
	threadPool                  *adapterthreadpool.Pool
	properties                  appconfig.Properties
	client                      *llm.Client
	sessionMemory               *memorysession.Store
	mongoDb                     *mongo.Client
	redisDb                     *redis.Client
	store                       persistenceport.Store
	agentRunRepository          agentrunport.Repository
	memoryStore                 memoryport.Store
	memoryCatalogService        memorycatalogapi.Service
	memoryDreamService          memorydreamapi.Service
	memoryExtractionService     *memoryextractionservice.Service
	memoryRetrievalService      memoryretrievalapi.ContextPreparer
	cache                       cacheport.CurrentSessionCache
	assetClient                 *asset.Client
	knowledgeBaseRepository     knowledgeport.KnowledgeBaseRepository
	knowledgeCategoryRepository knowledgeport.KnowledgeCategoryRepository
	documentRepository          knowledgeport.DocumentRepository
	chunkRepository             knowledgeport.ChunkRepository
	profileRepository           knowledgeport.ProfileRepository
	indexingJobRepository       knowledgeport.IndexingJobRepository
	indexingStateRepository     knowledgeport.IndexingStateRepository
	syncChangeRepository        knowledgeport.SyncChangeRepository
	documentObjectStore         knowledgeport.DocumentObjectStore
	documentProcessor           documentprocessorport.DocumentProcessor
	documentProcessorCloser     interface{ Close() error }
	embeddingModelRegistry      knowledgeport.EmbeddingModelRegistry
	embeddingModelResolver      knowledgeport.EmbeddingModelResolver
	vectorStore                 knowledgeport.VectorStore
	vectorStoreCloser           interface{ Close() error }
	localVectorStore            knowledgeport.VectorStore
	localVectorStoreCloser      interface{ Close() error }
	keywordStore                knowledgeport.KeywordStore
	keywordStoreCloser          interface{ Close() error }
	indexingService             indexingapi.Service
	retrievalService            retrievalapi.Service
	knowledgeCatalogService     catalogapi.Service
	knowledgeSearchService      searchapi.Service
	knowledgeQueryService       queryapi.Service
	knowledgeDocumentService    documentapi.Service
	knowledgeService            *service.KnowledgeService
	chatService                 *service.ChatService
	toolRegister                *tool.RegisterTools
	skillManager                *skill.Manager
	hookManager                 *hook.Manager
	mcpManager                  *mcp.Manager
	pluginManager               *pluginruntime.Manager
	localCommandExecutor        executionport.CommandExecutor
	isolatedSandboxManager      sandboxport.Manager
	subagentService             subagentapi.Service
	subagentScheduler           *subagentlocal.Scheduler
	subagentRegistry            *subagentmemory.Registry
	subagentEvents              *subagentevents.Bus
	workspaceIsolationManager   workspaceport.Manager
	workspaceCommandRunner      workspaceport.CommandRunner
	workspaceCloser             interface{ Close() error }
	defaultModelID              string
	workspace                   string
}

var (
	instance            *Application
	once                sync.Once
	configuredWorkspace string
)

func SetWorkspace(workspace string) {
	configuredWorkspace = workspace
}

func InitApp() {
	once.Do(func() {
		instance = &Application{workspace: configuredWorkspace}
		// 初始化顺序存在依赖关系：配置和基础设施必须先于工具注册与应用服务装配。
		instance.InitConfig()
		instance.InitAssetClient()
		instance.InitMongoDb()
		instance.InitRedisDb()
		//这里选取存储模式
		instance.InitStore()
		instance.InitThreadPool()
		instance.InitMemoryStorage()
		instance.InitKnowledgeStorage()
		instance.InitCache()
		//初始化模型信息，有哪些模型可以调用
		instance.InitClient()
		//一次性初始化 4 大记忆相关服务
		instance.InitMemoryServices()
		instance.InitSessionMemory()
		instance.InitSandbox()
		instance.InitWorkspaceIsolation()
		instance.InitSkillManager()
		instance.InitHookManager()
		instance.InitRegister()
		instance.InitMCP()
		instance.InitPlugins()
		instance.InitChatService()
		instance.InitSubagents()
	})
}

func GetApp() *Application {
	if instance == nil {
		panic("call core.InitApp() before core.GetApp()")
	}
	return instance
}

func (app *Application) InitConfig() {
	properties, err := (appconfig.ViperLoader{}).Load(app.workspace)
	if err != nil {
		panic(err)
	}
	app.properties = properties
}

func (app *Application) InitMongoDb() {
	uri := app.properties.Mongo.URI
	if uri == "" {
		return
	}

	client, err := infra.NewMongoClient(context.Background(), uri)
	if err != nil {
		panic(err)
	}

	app.mongoDb = client
}

func (app *Application) InitRedisDb() {
	properties := app.properties.Redis
	addr := properties.Address
	if addr == "" {
		return
	}

	client, err := infra.NewRedisClient(
		context.Background(),
		addr,
		properties.Password,
		properties.DB,
	)
	if err != nil {
		panic(err)
	}

	app.redisDb = client
}

func (app *Application) InitStore() {
	if app.mongoDb == nil {
		app.agentRunRepository = agentrunmemory.New()
		app.recoverAgentRuns()
		// Mongo 未配置时允许以内存模式启动，持久化相关适配器会保持为空。
		log.Print("agent run memory repository created")
		return
	}

	database := app.properties.Mongo.Database
	if database == "" {
		panic("mongo database not exist")
	}
	app.store = adaptermongo.New(app.mongoDb, database)
	app.agentRunRepository = agentrunmongo.New(app.mongoDb, database)
	app.recoverAgentRuns()
}

func (app *Application) recoverAgentRuns() {
	if app.agentRunRepository == nil {
		return
	}
	commands := agentrunservice.CommandService{Repository: app.agentRunRepository, IDs: uuidadapter.Generator{}}
	if err := commands.RecoverRunning(context.Background()); err != nil {
		log.Printf("recover interrupted agent runs failed: %v", err)
	}
}

func (app *Application) InitMemoryStorage() {
	if app.mongoDb == nil {
		//use memory to store
		app.memoryStore = aimemory.New()
		return
	}
	repository := memorymongo.New(app.mongoDb, app.properties.Mongo.Database)
	if err := repository.EnsureIndexes(context.Background()); err != nil {
		panic(fmt.Errorf("init AI memory indexes failed: %w", err))
	}
	app.memoryStore = repository
}

func (app *Application) InitMemoryServices() {
	if app.memoryStore == nil {
		return
	}
	ids := uuidadapter.Generator{}
	//初始化AI记忆存储服务，采用mongo和内存都可以
	app.memoryCatalogService = memorycatalogservice.CatalogService{Store: app.memoryStore, IDs: ids}
	//初始化当前问题相关的历史 AI 经验的服务
	app.memoryRetrievalService = memoryretrievalservice.ContextService{
		Memories: app.memoryStore,
		Usage:    app.memoryCatalogService,
		Scope:    app.defaultMemoryScope(),
		TopK:     4,
		OnUsageError: func(err error) {
			log.Printf("record AI memory use failed: %v", err)
		},
	}
	if app.client == nil {
		return
	}
	//选取记忆服务模型
	extractionModelID := strings.TrimSpace(app.properties.Memory.Extraction.ModelID)
	if extractionModelID == "" {
		extractionModelID = app.defaultModelID
	}
	model := app.client.GetModel(extractionModelID)
	if model == nil {
		log.Printf("AI memory extraction disabled: configured model %q is unavailable", extractionModelID)
	} else if app.agentRunRepository == nil {
		log.Printf("AI memory extraction disabled: agent run repository is unavailable")
	} else {
		//依赖全部就绪，实例化记忆提取服务
		extraction := &memoryextractionservice.Service{
			//  保存候选记忆和提取任务
			Store: app.memoryStore,
			//  读取 Agent 的运行记录和工具调用记录
			Runs: app.agentRunRepository,
			IDs:  ids,
			//	调用大模型，把运行记录总结成候选记忆
			Extractor: memoryextractor.ModelExtractor{
				ModelID:      extractionModelID,
				Model:        model,
				DefaultScope: app.defaultMemoryScope(),
			},
			Async: adapterthreadpool.Executor{Pool: app.threadPool},
			OnError: func(err error) {
				log.Printf("AI memory extraction failed: %v", err)
			},
		}
		app.memoryExtractionService = extraction
		if err := extraction.Recover(context.Background(), memoryextractioncommand.Recover{Limit: 100}); err != nil {
			log.Printf("recover AI memory extraction jobs failed: %v", err)
		}
	}

	dreamModelID := strings.TrimSpace(app.properties.Memory.Dream.ModelID)
	if dreamModelID == "" {
		dreamModelID = app.defaultModelID
	}
	dreamModel := app.client.GetModel(dreamModelID)
	if dreamModel == nil {
		log.Printf("AI memory dream disabled: configured model %q is unavailable", dreamModelID)
		return
	}
	app.memoryDreamService = &memorydreamservice.Service{
		Store: app.memoryStore, Catalog: app.memoryCatalogService, IDs: ids,
		Consolidator: memorydreamadapter.ModelConsolidator{Model: dreamModel},
	}
}

func (app *Application) defaultMemoryScope() domainmemory.Scope {
	workspace := strings.TrimSpace(app.workspace)
	if workspace == "" {
		return domainmemory.Scope{Type: domainmemory.ScopeGlobal}
	}
	if absolute, err := filepath.Abs(workspace); err == nil {
		workspace = filepath.Clean(absolute)
	}
	return domainmemory.Scope{Type: domainmemory.ScopeWorkspace, Key: workspace}
}

func (app *Application) InitKnowledgeStorage() {
	embeddingRegistry := embeddingmemory.NewRegistry()
	mapper := appconfig.Mapper{}
	for _, properties := range app.properties.RAG.Embedding.Models {
		info := mapper.EmbeddingModelInfo(properties)
		if !info.Enabled {
			continue
		}
		var provider knowledgeport.EmbeddingProvider
		var err error
		switch info.Provider {
		case "openai", "openai-compatible":
			provider, err = openaiembedding.New(mapper.EmbeddingProviderConfig(properties))
		default:
			err = fmt.Errorf("unsupported embedding provider %q", info.Provider)
		}
		if err != nil {
			panic(fmt.Errorf("init embedding model %q failed: %w", info.ID, err))
		}
		if err := embeddingRegistry.Set(info.ID, provider, info); err != nil {
			panic(fmt.Errorf("register embedding model %q failed: %w", info.ID, err))
		}
	}
	app.embeddingModelRegistry = embeddingRegistry
	app.embeddingModelResolver = embeddingservice.Resolver{Registry: embeddingRegistry}
	if app.properties.RAG.Milvus.Enabled {
		store, err := milvusadapter.New(context.Background(), mapper.MilvusConfig(app.properties.RAG.Milvus))
		if err != nil {
			panic(fmt.Errorf("init Milvus vector store failed: %w", err))
		}
		app.vectorStore = store
		app.vectorStoreCloser = store
	}

	if app.mongoDb != nil {
		database := app.properties.Mongo.Database
		app.knowledgeBaseRepository = knowledgemongo.NewKnowledgeBaseRepository(app.mongoDb, database)
		app.knowledgeCategoryRepository = knowledgemongo.NewKnowledgeCategoryRepository(app.mongoDb, database)
		app.documentRepository = knowledgemongo.NewDocumentRepository(app.mongoDb, database)
		app.chunkRepository = knowledgemongo.NewChunkRepository(app.mongoDb, database)
		app.profileRepository = knowledgemongo.NewProfileRepository(app.mongoDb, database)
		app.indexingJobRepository = knowledgemongo.NewIndexingJobRepository(app.mongoDb, database)
		app.indexingStateRepository = knowledgemongo.NewIndexingStateRepository(app.mongoDb, database)
		app.syncChangeRepository = knowledgemongo.NewSyncChangeRepository(app.mongoDb, database)
	}

	properties := app.properties.RAG.MinIO
	if strings.TrimSpace(properties.Endpoint) != "" {
		store, err := minioadapter.New((appconfig.Mapper{}).MinIOConfig(properties))
		if err != nil {
			panic(fmt.Errorf("init RAG MinIO failed: %w", err))
		}
		if err := store.EnsureBucket(context.Background()); err != nil {
			panic(fmt.Errorf("ensure RAG MinIO bucket failed: %w", err))
		}
		app.documentObjectStore = store
	}

	processorProperties := app.properties.RAG.DocumentProcessor
	if processorProperties.Enabled {
		processor, err := grpcprocessor.New(context.Background(), (appconfig.Mapper{}).DocumentProcessorConfig(processorProperties))
		if err != nil {
			panic(fmt.Errorf("init document processor failed: %w", err))
		}
		app.documentProcessor = processor
		app.documentProcessorCloser = processor
	}

	localProperties := app.properties.RAG.Local
	if localProperties.Enabled {
		keywordPath := localProperties.Path
		if strings.TrimSpace(keywordPath) == "" {
			var err error
			keywordPath, err = sqlitefts5.DefaultPath(app.workspace)
			if err != nil {
				panic(fmt.Errorf("resolve local knowledge database path failed: %w", err))
			}
		}
		keywordStore, err := sqlitefts5.Open(mapper.KeywordStoreConfig(localProperties, keywordPath))
		if err != nil {
			panic(fmt.Errorf("init SQLite FTS5 keyword store failed: %w", err))
		}
		vectorPath := localProperties.VectorPath
		if strings.TrimSpace(vectorPath) == "" {
			vectorPath, err = sqlitevec.PathBeside(keywordPath)
			if err != nil {
				_ = keywordStore.Close()
				panic(fmt.Errorf("resolve local vector database path failed: %w", err))
			}
		}
		vectorStore, err := sqlitevec.Open(mapper.LocalVectorStoreConfig(localProperties, vectorPath))
		if err != nil {
			_ = keywordStore.Close()
			panic(fmt.Errorf("init sqlite-vec vector store failed: %w", err))
		}
		app.keywordStore = keywordStore
		app.keywordStoreCloser = keywordStore
		app.localVectorStore = vectorStore
		app.localVectorStoreCloser = vectorStore
	}

	app.initIndexingService()
	app.initKnowledgeDocumentService()
	app.initRetrievalService()
	app.initKnowledgeCatalogService()
	app.initKnowledgeSearchService()
	app.initKnowledgeQueryService()
	app.initKnowledgeFacade()
}

func (app *Application) initKnowledgeDocumentService() {
	objectCleanup, _ := app.documentObjectStore.(knowledgeport.UncommittedDocumentObjectCleaner)
	if app.indexingService == nil || app.assetClient == nil || app.documentObjectStore == nil || objectCleanup == nil || app.knowledgeBaseRepository == nil || app.documentRepository == nil || app.chunkRepository == nil || app.indexingJobRepository == nil || app.indexingStateRepository == nil {
		return
	}
	ids, err := snowflakeid.New(app.properties.RAG.IDNode)
	if err != nil {
		panic(fmt.Errorf("init knowledge document id generator failed: %w", err))
	}
	documents := &documentservice.DocumentService{
		KnowledgeBases: app.knowledgeBaseRepository,
		Documents:      app.documentRepository,
		Chunks:         app.chunkRepository,
		Objects:        app.documentObjectStore,
		ObjectCleanup:  objectCleanup,
		Sources:        assetsource.Source{Client: app.assetClient},
		Indexing:       app.indexingService,
		Jobs:           app.indexingJobRepository,
		States:         app.indexingStateRepository,
		IDs:            ids,
		Async:          adapterthreadpool.Executor{Pool: app.threadPool},
		OnAsyncError: func(err error) {
			log.Printf("knowledge indexing background operation failed: %v", err)
		},
	}
	app.knowledgeDocumentService = documents
	if err := documents.RecoverPending(context.Background(), 1000); err != nil {
		log.Printf("recover knowledge indexing jobs failed: %v", err)
	}
}

func (app *Application) initKnowledgeCatalogService() {
	if app.knowledgeCategoryRepository == nil || app.knowledgeBaseRepository == nil {
		return
	}
	ids, err := snowflakeid.New(app.properties.RAG.IDNode)
	if err != nil {
		panic(fmt.Errorf("init knowledge catalog id generator failed: %w", err))
	}
	app.knowledgeCatalogService = catalogservice.CatalogService{
		Categories:     app.knowledgeCategoryRepository,
		KnowledgeBases: app.knowledgeBaseRepository,
		Profiles:       app.profileRepository,
		IDs:            ids,
	}
}

func (app *Application) initKnowledgeSearchService() {
	if app.knowledgeCategoryRepository == nil || app.knowledgeBaseRepository == nil || app.profileRepository == nil || app.retrievalService == nil {
		return
	}
	rrfK := app.properties.RAG.Retrieval.RRFK
	if rrfK == 0 {
		rrfK = 60
	}
	fusion, err := retrievalservice.NewReciprocalRankFusion(rrfK)
	if err != nil {
		panic(fmt.Errorf("init cross-profile knowledge search fusion failed: %w", err))
	}
	app.knowledgeSearchService = searchservice.SearchService{
		Categories:     app.knowledgeCategoryRepository,
		KnowledgeBases: app.knowledgeBaseRepository,
		Profiles:       app.profileRepository,
		Retrieval:      app.retrievalService,
		Fusion:         fusion,
	}
}

func (app *Application) initKnowledgeQueryService() {
	if app.knowledgeBaseRepository == nil || app.documentRepository == nil || app.profileRepository == nil {
		return
	}
	jobs, _ := app.indexingJobRepository.(knowledgeport.IndexingJobQueryRepository)
	profiles, _ := app.profileRepository.(knowledgeport.IndexProfileCatalog)
	app.knowledgeQueryService = queryservice.QueryService{
		KnowledgeBases:     app.knowledgeBaseRepository,
		DocumentRepository: app.documentRepository,
		Jobs:               jobs,
		Profiles:           profiles,
	}
}

func (app *Application) initKnowledgeFacade() {
	if app.knowledgeCatalogService == nil && app.knowledgeQueryService == nil && app.knowledgeSearchService == nil && app.knowledgeDocumentService == nil {
		return
	}
	app.knowledgeService = &service.KnowledgeService{
		Catalog:   app.knowledgeCatalogService,
		Query:     app.knowledgeQueryService,
		Search:    app.knowledgeSearchService,
		Documents: app.knowledgeDocumentService,
	}
}

func (app *Application) initIndexingService() {
	if app.documentRepository == nil || app.profileRepository == nil || app.indexingJobRepository == nil || app.indexingStateRepository == nil || app.chunkRepository == nil || app.documentObjectStore == nil || app.documentProcessor == nil || app.embeddingModelResolver == nil || app.vectorStore == nil || app.keywordStore == nil {
		return
	}
	jobIDs, err := snowflakeid.New(app.properties.RAG.IDNode)
	if err != nil {
		panic(fmt.Errorf("init indexing job id generator failed: %w", err))
	}
	service, err := indexingservice.New(indexingservice.Configuration{
		Documents:  app.documentRepository,
		Profiles:   app.profileRepository,
		Jobs:       app.indexingJobRepository,
		States:     app.indexingStateRepository,
		Chunks:     app.chunkRepository,
		Objects:    app.documentObjectStore,
		Processor:  app.documentProcessor,
		Embeddings: app.embeddingModelResolver,
		Vectors:    app.vectorStore,
		Keywords:   app.keywordStore,
		JobIDs:     jobIDs,
		StableIDs:  contenthashid.Deriver{},
	})
	if err != nil {
		panic(fmt.Errorf("init knowledge indexing service failed: %w", err))
	}
	app.indexingService = service
}

func (app *Application) initRetrievalService() {
	if app.knowledgeBaseRepository == nil || app.documentRepository == nil || app.profileRepository == nil || app.chunkRepository == nil || app.embeddingModelResolver == nil {
		return
	}
	if app.localVectorStore == nil && app.keywordStore == nil && app.vectorStore == nil {
		return
	}
	properties := app.properties.RAG.Retrieval
	service, err := retrievalservice.New(retrievalservice.Configuration{
		KnowledgeBases:      app.knowledgeBaseRepository,
		Documents:           app.documentRepository,
		Profiles:            app.profileRepository,
		Chunks:              app.chunkRepository,
		Embeddings:          app.embeddingModelResolver,
		LocalVectors:        app.localVectorStore,
		LocalKeywords:       app.keywordStore,
		RemoteVectors:       app.vectorStore,
		CandidateMultiplier: properties.CandidateMultiplier,
		MaxCandidates:       properties.MaxCandidates,
		MinLocalResults:     properties.MinLocalResults,
		MinLocalScore:       properties.MinLocalScore,
		RRFK:                properties.RRFK,
		CacheRemoteResults:  properties.CacheRemoteResults,
	})
	if err != nil {
		panic(fmt.Errorf("init knowledge retrieval service failed: %w", err))
	}
	app.retrievalService = service
}

func (app *Application) InitCache() {
	if app.redisDb == nil {
		// Redis 只保存“用户当前会话”等短期状态，不影响核心聊天流程启动。
		return
	}

	app.cache = adapterredis.NewCurrentSessionCache(app.redisDb)
}

func (app *Application) InitAssetClient() {
	properties := app.properties.Asset
	if properties.BaseURL == "" {
		return
	}

	client, err := asset.NewClient((appconfig.Mapper{}).AssetConfig(properties))
	if err != nil {
		panic(fmt.Errorf("init asset client failed: %w", err))
	}
	app.assetClient = client
}

func (app *Application) InitThreadPool() {
	properties := app.properties.Thread
	app.threadPool = adapterthreadpool.New(
		properties.Core,
		properties.QueueSize,
	)
}

func (app *Application) InitClient() {
	app.client = llm.NewClient()

	// 启动时先从配置和持久层加载模型，再把具体模型注册进运行时 Registry。
	result, err := (modelservice.BootstrapService{
		//读取 MongoDB 中保存的模型配置
		Repository: app.store,
		//保存的模型配置
		Registry: app.client,
		//协议模型保存
		Factory: adaptermodel.NewFactory(),
	}).Bootstrap(context.Background(), modelcommand.Bootstrap{
		Seed:            (appconfig.Mapper{}).ModelConfig(app.properties.Model),
		FallbackModelID: app.properties.Model.ID,
	})
	if err != nil {
		panic(err)
	}

	app.defaultModelID = result.DefaultModelID
}

func (app *Application) InitSessionMemory() {
	app.sessionMemory = memorysession.NewStore(app.defaultModelID)
}

func (app *Application) InitSandbox() {
	localExecutor, err := localexecutor.New(app.workspace)
	if err != nil {
		panic(fmt.Errorf("init local command executor failed: %w", err))
	}
	app.localCommandExecutor = localExecutor

	provider := strings.ToLower(strings.TrimSpace(app.properties.Sandbox.Provider))
	switch provider {
	case "", "local":
		return
	case "opensandbox":
		manager, err := opensandboxadapter.New((appconfig.Mapper{}).OpenSandboxConfig(app.properties.Sandbox.OpenSandbox))
		if err != nil {
			panic(fmt.Errorf("init OpenSandbox manager failed: %w", err))
		}
		app.isolatedSandboxManager = manager
	default:
		panic(fmt.Errorf("unsupported sandbox provider %q", provider))
	}
}

func (app *Application) InitWorkspaceIsolation() {
	//创建快照工作区管理器
	snapshotManager, err := snapshotworkspace.New(app.properties.Subagent.SnapshotRoot, sqlitehistory.Factory{})
	if err != nil {
		panic(fmt.Errorf("init snapshot workspace manager failed: %w", err))
	}
	//创建工作区路由器：
	router := &workspaceRouter.Manager{Snapshot: snapshotManager}
	if app.isolatedSandboxManager != nil {
		openSandboxManager, err := opensandboxworkspace.New(snapshotManager, app.isolatedSandboxManager)
		if err != nil {
			panic(fmt.Errorf("init OpenSandbox workspace manager failed: %w", err))
		}
		router.OpenSandbox = openSandboxManager
		router.Remote = openSandboxManager
	}
	app.workspaceIsolationManager = router
	app.workspaceCommandRunner = router
	app.workspaceCloser = router
}

func (app *Application) InitChatService() {
	// composition/chat 是显式依赖注入入口，相当于 Spring 的 @Configuration。
	app.chatService = chatcomposition.NewService(chatcomposition.Configuration{
		Models:           app.client,
		ModelFactory:     adaptermodel.NewFactory(),
		Sessions:         app.sessionMemory,
		Store:            app.store,
		Cache:            app.cache,
		Async:            adapterthreadpool.Executor{Pool: app.threadPool},
		Tools:            app.toolRegister,
		Skills:           app.skillManager,
		Hooks:            app.hookManager,
		DefaultModel:     app.defaultModelID,
		KnowledgeSearch:  app.knowledgeSearchService,
		AgentRuns:        app.agentRunRepository,
		AgentRunObserver: app.memoryExtractionService,
		MemoryContext:    app.memoryRetrievalService,
	})
	if err := app.chatService.Bootstrap(context.Background()); err != nil {
		panic(err)
	}
}

func (app *Application) InitSubagents() {
	registry := subagentmemory.NewRegistry()
	scheduler := subagentlocal.NewScheduler(
		app.properties.Subagent.WorkerCount,
		app.properties.Subagent.QueueSize,
	)

	var definitions subagentport.DefinitionRepository
	var tasks subagentport.TaskRepository
	var runs subagentport.RunRepository
	var taskEvents subagentport.TaskEventRepository
	if app.mongoDb != nil {
		repository := subagentmongo.New(app.mongoDb, app.properties.Mongo.Database)
		definitions = repository
		tasks = repository
		runs = repository
		taskEvents = repository
	} else {
		repository := subagentmemory.NewRepository()
		definitions = repository
		tasks = repository
		runs = repository
		taskEvents = repository
	}

	sessionPersistence := sessionpersistenceservice.PersistenceService{
		Sessions:     app.store,
		Memory:       app.sessionMemory,
		DefaultModel: app.defaultModelID,
	}
	applicationService := &subagentservice.Service{
		Definitions: definitions,
		Registry:    registry,
		Tasks:       tasks,
		Runs:        runs,
		TaskRuns:    tasks.(subagentport.TaskRunRepository),
		Scheduler:   scheduler,
		Sessions: subagentsession.Factory{
			Memory: app.sessionMemory, Persistence: sessionPersistence,
		},
		Runner:               subagentchat.Runner{Chat: app.chatService},
		ParentContinuation:   subagentchat.Continuation{Chat: app.chatService},
		IDs:                  uuidadapter.Generator{},
		DefaultWorkspaceRoot: app.workspace,
		OnError: func(err error) {
			log.Printf("subagent background operation failed: %v", err)
		},
	}
	eventBus := subagentevents.NewBus(taskEvents)
	applicationService.Events = eventBus
	applicationService.Workspaces = app.workspaceIsolationManager
	if _, err := applicationService.Bootstrap(context.Background(), subagentcommand.BootstrapDefinitions{}); err != nil {
		scheduler.Close()
		panic(fmt.Errorf("init subagents failed: %w", err))
	}
	if err := applicationService.RecoverInterruptedTasks(context.Background()); err != nil {
		scheduler.Close()
		panic(fmt.Errorf("recover interrupted subagent tasks failed: %w", err))
	}

	app.toolRegister.RegisterSource("subagent", local.NewSubagentTools(applicationService))
	app.subagentRegistry = registry
	app.subagentScheduler = scheduler
	app.subagentService = applicationService
	app.subagentEvents = eventBus
}

func (app *Application) Close() error {
	if app == nil {
		return nil
	}
	var errs []error
	// Stop task producers and drain workers before closing the resources used
	// by subagent, chat persistence, and knowledge indexing jobs.
	if app.subagentScheduler != nil {
		app.subagentScheduler.Close()
	}
	if app.threadPool != nil {
		app.threadPool.Shutdown()
	}
	if app.pluginManager != nil {
		errs = append(errs, app.pluginManager.Close())
	}
	if app.mcpManager != nil {
		errs = append(errs, app.mcpManager.Close())
	}
	if app.workspaceCloser != nil {
		errs = append(errs, app.workspaceCloser.Close())
	}
	if app.documentProcessorCloser != nil {
		errs = append(errs, app.documentProcessorCloser.Close())
	}
	if app.vectorStoreCloser != nil {
		errs = append(errs, app.vectorStoreCloser.Close())
	}
	if app.localVectorStoreCloser != nil {
		errs = append(errs, app.localVectorStoreCloser.Close())
	}
	if app.keywordStoreCloser != nil {
		errs = append(errs, app.keywordStoreCloser.Close())
	}
	return errors.Join(errs...)
}

func (app *Application) GetChatService() *service.ChatService {
	return app.chatService
}

func (app *Application) GetSandboxManager() sandboxport.Manager {
	return app.isolatedSandboxManager
}

func (app *Application) GetSubagentService() subagentapi.Service {
	return app.subagentService
}

func (app *Application) GetSubagentEvents() *subagentevents.Bus {
	return app.subagentEvents
}

func (app *Application) GetPluginManager() *pluginruntime.Manager {
	return app.pluginManager
}

func (app *Application) GetKnowledgeBaseRepository() knowledgeport.KnowledgeBaseRepository {
	return app.knowledgeBaseRepository
}

func (app *Application) GetKnowledgeCategoryRepository() knowledgeport.KnowledgeCategoryRepository {
	return app.knowledgeCategoryRepository
}

func (app *Application) GetDocumentRepository() knowledgeport.DocumentRepository {
	return app.documentRepository
}

func (app *Application) GetChunkRepository() knowledgeport.ChunkRepository {
	return app.chunkRepository
}

func (app *Application) GetProfileRepository() knowledgeport.ProfileRepository {
	return app.profileRepository
}

func (app *Application) GetIndexingJobRepository() knowledgeport.IndexingJobRepository {
	return app.indexingJobRepository
}

func (app *Application) GetSyncChangeRepository() knowledgeport.SyncChangeRepository {
	return app.syncChangeRepository
}

func (app *Application) GetDocumentObjectStore() knowledgeport.DocumentObjectStore {
	return app.documentObjectStore
}

func (app *Application) GetDocumentProcessor() documentprocessorport.DocumentProcessor {
	return app.documentProcessor
}

func (app *Application) GetEmbeddingModelRegistry() knowledgeport.EmbeddingModelRegistry {
	return app.embeddingModelRegistry
}

func (app *Application) GetEmbeddingModelResolver() knowledgeport.EmbeddingModelResolver {
	return app.embeddingModelResolver
}

func (app *Application) GetVectorStore() knowledgeport.VectorStore {
	return app.vectorStore
}

func (app *Application) GetLocalVectorStore() knowledgeport.VectorStore {
	return app.localVectorStore
}

func (app *Application) GetKeywordStore() knowledgeport.KeywordStore {
	return app.keywordStore
}

func (app *Application) GetIndexingService() indexingapi.Service {
	return app.indexingService
}

func (app *Application) GetRetrievalService() retrievalapi.Service {
	return app.retrievalService
}

func (app *Application) GetKnowledgeCatalogService() catalogapi.Service {
	return app.knowledgeCatalogService
}

func (app *Application) GetKnowledgeSearchService() searchapi.Service {
	return app.knowledgeSearchService
}

func (app *Application) GetKnowledgeService() *service.KnowledgeService {
	return app.knowledgeService
}

func (app *Application) GetMemoryCatalogService() memorycatalogapi.Service {
	return app.memoryCatalogService
}

func (app *Application) GetMemoryExtractionService() memoryextractionapi.Service {
	return app.memoryExtractionService
}

func (app *Application) GetMemoryDreamService() memorydreamapi.Service {
	return app.memoryDreamService
}

func (app *Application) InitRegister() *tool.RegisterTools {
	tools := tool.NewRegisterTools()
	// 所有本地工具都在这里集中注册；业务层只通过 ToolCatalog/ToolExecutor 接口使用它们。
	localTools := []tooldef.Tool{
		local.NewListFilesToolWithWorkspace(app.workspace),
		local.NewReadFileToolWithWorkspace(app.workspace),
		local.NewSearchFilesToolWithWorkspace(app.workspace),
		local.NewWriteFileToolWithWorkspace(app.workspace),
		local.NewEditFileToolWithWorkspace(app.workspace),
		local.NewShellToolWithWorkspaceAndRemote(app.workspace, app.localCommandExecutor, app.workspaceCommandRunner),
		local.NewInstallSkillToolWithWorkspaceRegistryHooksAndSkills(app.workspace, app.skillRoot(), app.skillHubRegistry(), app.hookManager, app.skillManager),
	}
	if app.assetClient != nil {
		localTools = append(localTools, local.NewReadAssetToolWithDownloader(app.assetClient))
		localTools = append(localTools, local.NewShareFileToolWithWorkspaceAndUploader(app.workspace, app.assetClient))
	}
	if app.knowledgeSearchService != nil {
		localTools = append(localTools, local.NewKnowledgeSearchTool(app.knowledgeSearchService))
	}
	tools.RegisterSource("local", localTools)
	app.toolRegister = tools
	return tools
}

func (app *Application) InitMCP() *mcp.Manager {
	config := (appconfig.Mapper{}).MCPConfig(app.workspace, app.properties.MCP)
	app.mcpManager = mcp.NewManager(config)
	if len(config.Servers) == 0 {
		return app.mcpManager
	}

	// MCP 工具最终进入同一个工具注册表，因此权限、Hook 和执行记录可复用本地工具链路。
	if err := app.mcpManager.RegisterAll(context.Background(), app.toolRegister); err != nil {
		panic(fmt.Errorf("init mcp failed: %w", err))
	}
	return app.mcpManager
}

func (app *Application) InitPlugins() *pluginruntime.Manager {
	manager := pluginruntime.NewManager(app.properties.Plugin.Root)
	app.pluginManager = manager
	if !app.properties.Plugin.Enabled {
		return manager
	}
	if err := manager.Load(context.Background(), app.toolRegister); err != nil {
		panic(fmt.Errorf("init plugins failed: %w", err))
	}
	return manager
}

func (app *Application) InitSkillManager() *skill.Manager {
	app.skillManager = skill.NewManager(app.skillRoot())
	return app.skillManager
}

func (app *Application) InitHookManager() *hook.Manager {
	manager, err := hook.NewManager((appconfig.Mapper{}).HookConfig(app.workspace, app.properties.Hooks))
	if err != nil {
		panic(fmt.Errorf("init hooks failed: %w", err))
	}
	app.hookManager = manager
	return app.hookManager
}

func (app *Application) skillRoot() string {
	return app.properties.Skill.Root
}

func (app *Application) skillHubRegistry() string {
	return app.properties.Skill.Registry
}
