package service

import (
	"context"
	"errors"
	"fmt"

	modelapi "myai/core/application/model/api"
	modelcommand "myai/core/application/model/command"
	modelresult "myai/core/application/model/result"
	generation "myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

type BootstrapService struct {
	// 启动时把持久化模型配置实例化为运行时模型，并注册到 Registry。
	//读取 MongoDB 中保存的模型配置
	Repository modelport.ConfigRepository
	//保存启动后可以使用的模型对象
	Registry modelport.MutableRegistry
	//根据协议创建具体模型实现
	Factory modelport.Factory
}

var _ modelapi.BootstrapService = BootstrapService{}

func (s BootstrapService) Bootstrap(ctx context.Context, command modelcommand.Bootstrap) (modelresult.Bootstrap, error) {
	if s.Registry == nil {
		return modelresult.Bootstrap{}, errors.New("llm client is nil")
	}
	if s.Factory == nil {
		return modelresult.Bootstrap{}, errors.New("model factory is nil")
	}
	if command.Seed.ID != "" {
		if err := generation.Validate(command.Seed.DefaultGenerationSettings); err != nil {
			return modelresult.Bootstrap{}, fmt.Errorf("invalid generation settings for seed model %s: %w", command.Seed.ID, err)
		}
	}

	// 数据库配置优先；为空时 loadConfigs 会使用配置文件提供的 seed 模型。
	configs, err := s.loadConfigs(ctx, command.Seed)
	if err != nil {
		return modelresult.Bootstrap{}, err
	}
	//选取默认的模型
	defaultModelID := DefaultModelID(configs, command.FallbackModelID)
	//进行获取到的所有模型进行校验，才能进入到模型配置里面去
	for _, config := range configs {
		config = normalizeModelConfig(config)
		modelID := config.ID
		if err := generation.Validate(config.DefaultGenerationSettings); err != nil {
			return modelresult.Bootstrap{}, fmt.Errorf("invalid generation settings for model %s: %w", modelID, err)
		}
		modelName := config.ModelName
		if modelName == "" {
			modelName = modelID
		}

		var model modelport.ChatModelPort
		if config.Enabled {
			model, err = s.Factory.CreateModel(modelport.CreationConfig{
				Provider:  config.Provider,
				Protocol:  config.Protocol,
				AuthType:  config.AuthType,
				APIKey:    config.APIKey,
				BaseURL:   config.BaseURL,
				ModelName: modelName,
			})
			if err != nil {
				return modelresult.Bootstrap{}, fmt.Errorf("create model %s failed: %w", modelID, err)
			}
		}

		s.Registry.SetModelInfo(modelID, model, modelport.ModelInfo{
			ID:                        modelID,
			Name:                      config.Name,
			Provider:                  config.Provider,
			Protocol:                  config.Protocol,
			AuthType:                  config.AuthType,
			BaseURL:                   config.BaseURL,
			HasAPIKey:                 config.APIKey != "",
			ModelName:                 modelName,
			Enabled:                   config.Enabled,
			IsDefault:                 config.IsDefault || modelID == defaultModelID,
			DefaultGenerationSettings: generation.Clone(config.DefaultGenerationSettings),
		})
	}

	if defaultModelID == "" || !s.Registry.HasModel(defaultModelID) {
		return modelresult.Bootstrap{}, errors.New("no enabled model urlConfig")
	}

	return modelresult.Bootstrap{
		Configs:        configs,
		DefaultModelID: defaultModelID,
	}, nil
}

func (s BootstrapService) loadConfigs(ctx context.Context, seed domainmodel.Config) ([]domainmodel.Config, error) {
	if s.Repository != nil {
		configs, err := s.Repository.ListConfigs(ctx)
		if err != nil {
			return nil, err
		}
		if len(configs) > 0 {
			return configs, nil
		}
		if seed.ID != "" {
			if err := s.Repository.SaveConfig(ctx, seed); err != nil {
				return nil, err
			}
			return []domainmodel.Config{seed}, nil
		}
	}

	if seed.ID == "" {
		return nil, errors.New("model urlConfig is empty")
	}
	return []domainmodel.Config{seed}, nil
}

func DefaultModelID(models []domainmodel.Config, fallback string) string {
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
