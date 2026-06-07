package core

import (
	"context"
	"fmt"
	"myai/core/sandbox"
	"myai/core/tool"
	"myai/core/tool/local"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"myai/core/infra"
	"myai/core/llm"
	"myai/core/service"
	"myai/core/session"
	"myai/core/store/cache"
	"myai/core/store/cache/redisCache"
	"myai/core/store/data"
	"myai/core/store/data/mongoDb"
	"myai/utills"
)

const (
	configThreadCoreKey  = "thread.core"
	configThreadQueueKey = "thread.queueSize"
)

type Application struct {
	threadPool     *utills.ThreadPool
	viper          *viper.Viper
	client         *llm.Client
	sessionManage  *session.SessionManage
	mongoDb        *mongo.Client
	redisDb        *redis.Client
	store          data.Store
	cache          cache.Cache
	chatService    *service.ChatService
	toolRegister   *tool.RegisterTools
	sandbox        sandbox.Sandbox
	defaultModelID string
}

var (
	instance *Application
	once     sync.Once
)

func InitApp() {
	once.Do(func() {
		instance = &Application{}
		instance.InitViper()
		instance.InitMongoDb()
		instance.InitRedisDb()
		instance.InitStore()
		instance.InitCache()
		instance.InitThreadPool()
		instance.InitClient()
		instance.InitSessionManage()
		instance.InitSandbox()
		instance.InitRegister()
		instance.InitChatService()
	})
}

func GetApp() *Application {
	if instance == nil {
		panic("call core.InitApp() before core.GetApp()")
	}
	return instance
}

func (app *Application) InitViper() {
	app.viper = viper.New()
	app.viper.SetConfigFile("./resource/application.yaml")
	err := app.viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
}

func (app *Application) InitMongoDb() {
	uri := app.viper.GetString("mongo.uri")
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
	addr := app.viper.GetString("redis.addr")
	if addr == "" {
		return
	}

	client, err := infra.NewRedisClient(
		context.Background(),
		addr,
		app.viper.GetString("redis.password"),
		app.viper.GetInt("redis.db"),
	)
	if err != nil {
		panic(err)
	}

	app.redisDb = client
}

func (app *Application) InitStore() {
	if app.mongoDb == nil {
		return
	}

	database := app.viper.GetString("mongo.database")
	app.store = mongoDb.New(app.mongoDb, database)
}

func (app *Application) InitCache() {
	if app.redisDb == nil {
		return
	}

	app.cache = redisCache.New(app.redisDb)
}

func (app *Application) InitThreadPool() {
	app.threadPool = utills.NewThreadPool(
		app.viper.GetInt(configThreadCoreKey),
		app.viper.GetInt(configThreadQueueKey),
	)
}

func (app *Application) InitClient() {
	app.client = llm.NewClient()

	models, err := app.loadModelConfigs(context.Background())
	if err != nil {
		panic(err)
	}

	app.defaultModelID = defaultModelID(models, app.viper.GetString("myai.model"))
	for _, config := range models {
		if !config.Enabled {
			continue
		}

		modelID := config.ID
		modelName := config.ModelName
		if modelName == "" {
			modelName = modelID
		}

		model, err := utills.CreateLLM(config.APIKey, config.BaseURL, modelName)
		if err != nil {
			panic(fmt.Errorf("create model %s failed: %w", modelID, err))
		}

		app.client.SetModelInfo(modelID, model, llm.ModelInfo{
			ID:        modelID,
			Name:      config.Name,
			Provider:  config.Provider,
			ModelName: modelName,
			Enabled:   config.Enabled,
			IsDefault: config.IsDefault || modelID == app.defaultModelID,
		})
	}

	if len(app.client.ListModels()) == 0 {
		panic("no enabled model config")
	}
}

func (app *Application) InitSessionManage() {
	app.sessionManage = session.NewSessionManage(app.defaultModelID)
}

func (app *Application) InitSandbox() {
	localSandbox, err := sandbox.NewLocalSandbox("")
	if err != nil {
		panic(err)
	}
	app.sandbox = localSandbox
}

func (app *Application) InitChatService() {
	app.chatService = service.NewChatService(
		app.client,
		app.sessionManage,
		app.store,
		app.cache,
		app.threadPool,
		app.toolRegister,
		app.defaultModelID,
	)
	if err := app.chatService.Bootstrap(context.Background()); err != nil {
		panic(err)
	}
}

func (app *Application) GetThreadPool() *utills.ThreadPool {
	return app.threadPool
}

func (app *Application) GetViper() *viper.Viper {
	return app.viper
}

func (app *Application) GetClient() *llm.Client {
	return app.client
}

func (app *Application) GetSessionManage() *session.SessionManage {
	return app.sessionManage
}

func (app *Application) GetMongoDb() *mongo.Client {
	return app.mongoDb
}

func (app *Application) GetRedisDb() *redis.Client {
	return app.redisDb
}

func (app *Application) GetStore() data.Store {
	return app.store
}

func (app *Application) GetCache() cache.Cache {
	return app.cache
}

func (app *Application) GetChatService() *service.ChatService {
	return app.chatService
}

func (app *Application) InitRegister() *tool.RegisterTools {
	tools := tool.NewRegisterTools()
	tools.Register(local.NewListFilesTool())
	tools.Register(local.NewReadFileTool())
	tools.Register(local.NewSearchFilesTool())
	tools.Register(local.NewWriteFileTool())
	tools.Register(local.NewEditFileTool())
	tools.Register(local.NewShellTool(app.sandbox))
	app.toolRegister = tools
	return tools
}

func (app *Application) GetToolRegister() *tool.RegisterTools {
	return app.toolRegister
}

func (app *Application) loadModelConfigs(ctx context.Context) ([]data.ModelConfig, error) {
	if app.store != nil {
		configs, err := app.store.ListModelConfigs(ctx)
		if err != nil {
			return nil, err
		}
		if len(configs) > 0 {
			return configs, nil
		}

		seed := app.modelConfigFromViper()
		if seed.ID != "" {
			if err := app.store.SaveModelConfig(ctx, seed); err != nil {
				return nil, err
			}
			return []data.ModelConfig{seed}, nil
		}
	}

	seed := app.modelConfigFromViper()
	if seed.ID == "" {
		return nil, fmt.Errorf("model config is empty")
	}
	return []data.ModelConfig{seed}, nil
}

func (app *Application) modelConfigFromViper() data.ModelConfig {
	modelID := app.viper.GetString("myai.model")
	if modelID == "" {
		modelID = "gpt-5.5"
	}

	now := time.Now()
	return data.ModelConfig{
		ID:        modelID,
		Name:      modelID,
		Provider:  "openai",
		BaseURL:   app.viper.GetString("myai.base_url"),
		APIKey:    app.viper.GetString("myai.api_key"),
		ModelName: modelID,
		Enabled:   true,
		IsDefault: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func defaultModelID(models []data.ModelConfig, fallback string) string {
	for _, model := range models {
		if model.Enabled && model.IsDefault && model.ID != "" {
			return model.ID
		}
	}

	if fallback != "" {
		for _, model := range models {
			if model.Enabled && model.ID == fallback {
				return fallback
			}
		}
	}

	for _, model := range models {
		if model.Enabled && model.ID != "" {
			return model.ID
		}
	}

	return fallback
}
