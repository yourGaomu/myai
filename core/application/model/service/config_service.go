package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	modelapi "myai/core/application/model/api"
	modelcommand "myai/core/application/model/command"
	modelresult "myai/core/application/model/result"
	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

type ConfigService struct {
	// 新模型必须先通过 Factory 创建成功并持久化，最后才进入运行时 Registry。
	Repository modelport.ConfigWriter
	Registry   modelport.MutableRegistry
	Factory    modelport.Factory
	Now        func() time.Time
}

var _ modelapi.ConfigService = ConfigService{}

func (s ConfigService) AddConfig(ctx context.Context, command modelcommand.AddConfig) (modelresult.AddConfig, error) {
	if s.Repository == nil {
		return modelresult.AddConfig{}, errors.New("model store is nil")
	}
	if s.Registry == nil {
		return modelresult.AddConfig{}, errors.New("llm client is nil")
	}
	if s.Factory == nil {
		return modelresult.AddConfig{}, errors.New("model factory is nil")
	}

	config := normalizeModelConfig(domainmodel.Config{
		ID:        command.ID,
		Name:      command.Name,
		Provider:  command.Provider,
		BaseURL:   command.BaseURL,
		APIKey:    command.APIKey,
		ModelName: command.ModelName,
		IsDefault: command.IsDefault,
	})
	if err := validateModelID(config.ID); err != nil {
		return modelresult.AddConfig{}, err
	}
	if s.Registry.HasModel(config.ID) {
		return modelresult.AddConfig{}, fmt.Errorf("model already exists: %s", config.ID)
	}
	config, err := s.prepareNewConfig(config)
	if err != nil {
		return modelresult.AddConfig{}, err
	}

	model, err := s.Factory.CreateModel(modelport.CreationConfig{
		Provider:  config.Provider,
		APIKey:    config.APIKey,
		BaseURL:   config.BaseURL,
		ModelName: config.ModelName,
	})
	if err != nil {
		return modelresult.AddConfig{}, err
	}

	if err := s.Repository.SaveConfig(ctx, config); err != nil {
		return modelresult.AddConfig{}, err
	}

	s.Registry.SetModelInfo(config.ID, model, modelport.ModelInfo{
		ID:        config.ID,
		Name:      config.Name,
		Provider:  config.Provider,
		ModelName: config.ModelName,
		Enabled:   config.Enabled,
		IsDefault: config.IsDefault,
	})
	return modelresult.AddConfig{Config: config}, nil
}

func (s ConfigService) prepareNewConfig(config domainmodel.Config) (domainmodel.Config, error) {
	config = normalizeModelConfig(config)
	if err := validateModelID(config.ID); err != nil {
		return domainmodel.Config{}, err
	}
	if config.Name == "" {
		config.Name = config.ID
	}
	if config.Provider == "" {
		config.Provider = "openai"
	}
	if config.Provider != "openai" && config.Provider != "openai-compatible" {
		return domainmodel.Config{}, fmt.Errorf("unsupported provider: %s", config.Provider)
	}
	if config.BaseURL == "" {
		return domainmodel.Config{}, errors.New("base url is empty")
	}
	if config.APIKey == "" {
		return domainmodel.Config{}, errors.New("api key is empty")
	}
	if config.ModelName == "" {
		config.ModelName = config.ID
	}

	now := s.now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	config.Enabled = true
	return config, nil
}

func normalizeModelConfig(config domainmodel.Config) domainmodel.Config {
	config.ID = strings.TrimSpace(config.ID)
	config.Name = strings.TrimSpace(config.Name)
	config.Provider = strings.TrimSpace(strings.ToLower(config.Provider))
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.ModelName = strings.TrimSpace(config.ModelName)
	return config
}

func validateModelID(modelID string) error {
	if modelID == "" {
		return errors.New("model id is empty")
	}
	if strings.ContainsAny(modelID, " \t\r\n") {
		return errors.New("model id cannot contain spaces")
	}
	return nil
}

func (s ConfigService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
