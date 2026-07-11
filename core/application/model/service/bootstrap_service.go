package service

import (
	"context"
	"errors"
	"fmt"

	modelapi "myai/core/application/model/api"
	modelcommand "myai/core/application/model/command"
	modelresult "myai/core/application/model/result"
	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

type BootstrapService struct {
	// 启动时把持久化模型配置实例化为运行时模型，并注册到 Registry。
	Repository modelport.ConfigRepository
	Registry   modelport.MutableRegistry
	Factory    modelport.Factory
}

var _ modelapi.BootstrapService = BootstrapService{}

func (s BootstrapService) Bootstrap(ctx context.Context, command modelcommand.Bootstrap) (modelresult.Bootstrap, error) {
	if s.Registry == nil {
		return modelresult.Bootstrap{}, errors.New("llm client is nil")
	}
	if s.Factory == nil {
		return modelresult.Bootstrap{}, errors.New("model factory is nil")
	}

	// 数据库配置优先；为空时 loadConfigs 会使用配置文件提供的 seed 模型。
	configs, err := s.loadConfigs(ctx, command.Seed)
	if err != nil {
		return modelresult.Bootstrap{}, err
	}

	defaultModelID := DefaultModelID(configs, command.FallbackModelID)
	for _, config := range configs {
		if !config.Enabled {
			continue
		}
		modelID := config.ID
		modelName := config.ModelName
		if modelName == "" {
			modelName = modelID
		}

		model, err := s.Factory.CreateModel(modelport.CreationConfig{
			Provider:  config.Provider,
			APIKey:    config.APIKey,
			BaseURL:   config.BaseURL,
			ModelName: modelName,
		})
		if err != nil {
			return modelresult.Bootstrap{}, fmt.Errorf("create model %s failed: %w", modelID, err)
		}

		s.Registry.SetModelInfo(modelID, model, modelport.ModelInfo{
			ID:        modelID,
			Name:      config.Name,
			Provider:  config.Provider,
			ModelName: modelName,
			Enabled:   config.Enabled,
			IsDefault: config.IsDefault || modelID == defaultModelID,
		})
	}

	if len(s.Registry.ListModels()) == 0 {
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
