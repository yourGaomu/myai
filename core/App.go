package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	adapterthreadpool "myai/core/adapter/async/threadpool"
	adapterredis "myai/core/adapter/cache/redis"
	adaptermodel "myai/core/adapter/model/langchaingo"
	adaptermongo "myai/core/adapter/persistence/mongo"
	memorysession "myai/core/adapter/session/memory"
	modelcommand "myai/core/application/model/command"
	modelservice "myai/core/application/model/service"
	"myai/core/asset"
	chatcomposition "myai/core/composition/chat"
	appconfig "myai/core/config"
	"myai/core/hook"
	"myai/core/infra"
	"myai/core/llm"
	"myai/core/mcp"
	cacheport "myai/core/port/cache"
	persistenceport "myai/core/port/persistence"
	"myai/core/sandbox"
	"myai/core/service"
	"myai/core/skill"
	"myai/core/tool"
	"myai/core/tool/local"
	tooldef "myai/core/tool/tool"
)

type Application struct {
	// Application 是进程级资源容器，作用类似 Spring Boot 的 ApplicationContext。
	// 它只负责创建和持有基础设施，不承载聊天、Plan 等业务规则。
	threadPool     *adapterthreadpool.Pool
	properties     appconfig.Properties
	client         *llm.Client
	sessionMemory  *memorysession.Store
	mongoDb        *mongo.Client
	redisDb        *redis.Client
	store          persistenceport.Store
	cache          cacheport.CurrentSessionCache
	assetClient    *asset.Client
	chatService    *service.ChatService
	toolRegister   *tool.RegisterTools
	skillManager   *skill.Manager
	hookManager    *hook.Manager
	mcpManager     *mcp.Manager
	sandbox        sandbox.Sandbox
	defaultModelID string
	workspace      string
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
		instance.InitStore()
		instance.InitCache()
		instance.InitThreadPool()
		instance.InitClient()
		instance.InitSessionMemory()
		instance.InitSandbox()
		instance.InitSkillManager()
		instance.InitHookManager()
		instance.InitRegister()
		instance.InitMCP()
		instance.InitChatService()
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
		// Mongo 未配置时允许以内存模式启动，持久化相关适配器会保持为空。
		return
	}

	database := app.properties.Mongo.Database
	app.store = adaptermongo.New(app.mongoDb, database)
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
		Repository: app.store,
		Registry:   app.client,
		Factory:    adaptermodel.Factory{},
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
	localSandbox, err := sandbox.NewLocalSandbox(app.workspace)
	if err != nil {
		panic(err)
	}
	app.sandbox = localSandbox
}

func (app *Application) InitChatService() {
	// composition/chat 是显式依赖注入入口，相当于 Spring 的 @Configuration。
	app.chatService = chatcomposition.NewService(chatcomposition.Configuration{
		Models:       app.client,
		ModelFactory: adaptermodel.Factory{},
		Sessions:     app.sessionMemory,
		Store:        app.store,
		Cache:        app.cache,
		Async:        adapterthreadpool.Executor{Pool: app.threadPool},
		Tools:        app.toolRegister,
		Skills:       app.skillManager,
		Hooks:        app.hookManager,
		DefaultModel: app.defaultModelID,
	})
	if err := app.chatService.Bootstrap(context.Background()); err != nil {
		panic(err)
	}
}

func (app *Application) Close() error {
	if app == nil {
		return nil
	}
	var errs []error
	if app.mcpManager != nil {
		errs = append(errs, app.mcpManager.Close())
	}
	if app.threadPool != nil {
		app.threadPool.Shutdown()
	}
	return errors.Join(errs...)
}

func (app *Application) GetClient() *llm.Client {
	return app.client
}

func (app *Application) GetSessionMemory() *memorysession.Store {
	return app.sessionMemory
}

func (app *Application) GetMongoDb() *mongo.Client {
	return app.mongoDb
}

func (app *Application) GetRedisDb() *redis.Client {
	return app.redisDb
}

func (app *Application) GetStore() persistenceport.Store {
	return app.store
}

func (app *Application) GetCache() cacheport.CurrentSessionCache {
	return app.cache
}

func (app *Application) GetAssetClient() *asset.Client {
	return app.assetClient
}

func (app *Application) GetChatService() *service.ChatService {
	return app.chatService
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
		local.NewShellToolWithWorkspace(app.workspace, app.sandbox),
		local.NewInstallSkillToolWithWorkspaceRegistryHooksAndSkills(app.workspace, app.skillRoot(), app.skillHubRegistry(), app.hookManager, app.skillManager),
	}
	if app.assetClient != nil {
		localTools = append(localTools, local.NewReadAssetToolWithDownloader(app.assetClient))
		localTools = append(localTools, local.NewShareFileToolWithWorkspaceAndUploader(app.workspace, app.assetClient))
	}
	tools.RegisterSource("local", localTools)
	app.toolRegister = tools
	return tools
}

func (app *Application) GetToolRegister() *tool.RegisterTools {
	return app.toolRegister
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

func (app *Application) GetMCPManager() *mcp.Manager {
	return app.mcpManager
}

func (app *Application) InitSkillManager() *skill.Manager {
	app.skillManager = skill.NewManager(app.skillRoot())
	return app.skillManager
}

func (app *Application) InitHookManager() *hook.Manager {
	app.hookManager = hook.NewManager((appconfig.Mapper{}).HookConfig(app.workspace, app.properties.Hooks))
	return app.hookManager
}

func (app *Application) skillRoot() string {
	return app.properties.Skill.Root
}

func (app *Application) skillHubRegistry() string {
	return app.properties.Skill.Registry
}

func (app *Application) GetSkillManager() *skill.Manager {
	return app.skillManager
}
