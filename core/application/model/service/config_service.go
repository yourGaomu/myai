package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	modelapi "myai/core/application/model/api"
	modelcommand "myai/core/application/model/command"
	modelappport "myai/core/application/model/port"
	modelresult "myai/core/application/model/result"
	generation "myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
)

type ConfigService struct {
	// 新模型必须先通过 Factory 创建成功并持久化，最后才进入运行时 Registry。
	Repository modelport.ConfigRepository
	Registry   modelport.MutableRegistry
	Factory    modelport.Factory
	Usage      modelappport.UsageChecker
	Default    modelappport.DefaultModelUpdater
	Now        func() time.Time
}

var _ modelapi.ConfigService = ConfigService{}

func (s ConfigService) AddConfig(ctx context.Context, command modelcommand.AddConfig) (modelresult.AddConfig, error) {
	if err := s.validateDependencies(); err != nil {
		return modelresult.AddConfig{}, err
	}

	config := normalizeModelConfig(domainmodel.Config{
		ID:                        command.ID,
		Name:                      command.Name,
		Provider:                  command.Provider,
		Protocol:                  command.Protocol,
		AuthType:                  command.AuthType,
		BaseURL:                   command.BaseURL,
		APIKey:                    command.APIKey,
		ModelName:                 command.ModelName,
		IsDefault:                 command.IsDefault,
		DefaultGenerationSettings: generation.Clone(command.DefaultGenerationSettings),
	})
	if err := validateModelID(config.ID); err != nil {
		return modelresult.AddConfig{}, err
	}
	if existing, err := s.Repository.GetConfig(ctx, config.ID); err == nil && existing.ID != "" {
		return modelresult.AddConfig{}, fmt.Errorf("model already exists: %s", config.ID)
	} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return modelresult.AddConfig{}, err
	}
	if s.Registry.HasModel(config.ID) {
		return modelresult.AddConfig{}, fmt.Errorf("model already exists: %s", config.ID)
	}
	config, err := s.prepareNewConfig(config)
	if err != nil {
		return modelresult.AddConfig{}, err
	}

	model, err := s.createModel(config)
	if err != nil {
		return modelresult.AddConfig{}, err
	}

	if err := s.Repository.SaveConfig(ctx, config); err != nil {
		return modelresult.AddConfig{}, err
	}

	s.Registry.SetModelInfo(config.ID, model, modelInfo(config))
	return modelresult.AddConfig{Config: config}, nil
}

func (s ConfigService) UpdateConfig(ctx context.Context, command modelcommand.UpdateConfig) (modelresult.ConfigMutation, error) {
	if err := s.validateDependencies(); err != nil {
		return modelresult.ConfigMutation{}, err
	}
	old, err := s.Repository.GetConfig(ctx, strings.TrimSpace(command.ID))
	if err != nil {
		return modelresult.ConfigMutation{}, err
	}
	apiKey := strings.TrimSpace(command.APIKey)
	if apiKey == "" && domainmodel.NormalizeAuthType(command.AuthType) != domainmodel.AuthTypeNone {
		apiKey = old.APIKey
	}
	config := normalizeModelConfig(domainmodel.Config{
		ID: old.ID, Name: command.Name, Provider: command.Provider,
		Protocol: command.Protocol, AuthType: command.AuthType, BaseURL: command.BaseURL,
		APIKey: apiKey, ModelName: command.ModelName, Enabled: old.Enabled,
		IsDefault: old.IsDefault, CreatedAt: old.CreatedAt,
		DefaultGenerationSettings: generation.Clone(command.DefaultGenerationSettings),
	})
	config, err = s.prepareUpdatedConfig(config, old)
	if err != nil {
		return modelresult.ConfigMutation{}, err
	}
	var model modelport.ChatModelPort
	if config.Enabled {
		model, err = s.createModel(config)
		if err != nil {
			return modelresult.ConfigMutation{}, err
		}
	}
	if err := s.Repository.SaveConfig(ctx, config); err != nil {
		return modelresult.ConfigMutation{}, err
	}
	s.Registry.SetModelInfo(config.ID, model, modelInfo(config))
	return modelresult.ConfigMutation{Config: config, Message: fmt.Sprintf("Model %s updated.", config.ID)}, nil
}

func (s ConfigService) SetEnabled(ctx context.Context, command modelcommand.SetEnabled) (modelresult.ConfigMutation, error) {
	if err := s.validateDependencies(); err != nil {
		return modelresult.ConfigMutation{}, err
	}
	config, err := s.Repository.GetConfig(ctx, strings.TrimSpace(command.ID))
	if err != nil {
		return modelresult.ConfigMutation{}, err
	}
	if config.Enabled == command.Enabled {
		return modelresult.ConfigMutation{Config: config, Message: fmt.Sprintf("Model %s is already %s.", config.ID, enabledLabel(config.Enabled))}, nil
	}
	if !command.Enabled {
		if config.IsDefault {
			return modelresult.ConfigMutation{}, errors.New("cannot disable the default model; select another default model first")
		}
		inUse, err := s.isModelInUse(ctx, config.ID)
		if err != nil {
			return modelresult.ConfigMutation{}, err
		}
		if inUse {
			return modelresult.ConfigMutation{}, fmt.Errorf("cannot disable model %s because it is used by a session", config.ID)
		}
	}
	config.Enabled = command.Enabled
	config.UpdatedAt = s.now()
	var model modelport.ChatModelPort
	if config.Enabled {
		model, err = s.createModel(config)
		if err != nil {
			return modelresult.ConfigMutation{}, err
		}
	}
	if err := s.Repository.SaveConfig(ctx, config); err != nil {
		return modelresult.ConfigMutation{}, err
	}
	s.Registry.SetModelInfo(config.ID, model, modelInfo(config))
	return modelresult.ConfigMutation{Config: config, Message: fmt.Sprintf("Model %s %s.", config.ID, enabledLabel(config.Enabled))}, nil
}

func (s ConfigService) SetDefault(ctx context.Context, command modelcommand.SetDefault) (modelresult.ConfigMutation, error) {
	if err := s.validateDependencies(); err != nil {
		return modelresult.ConfigMutation{}, err
	}
	modelID := strings.TrimSpace(command.ID)
	configs, err := s.Repository.ListConfigs(ctx)
	if err != nil {
		return modelresult.ConfigMutation{}, err
	}
	selectedIndex := -1
	for index := range configs {
		configs[index] = normalizeModelConfig(configs[index])
		if configs[index].ID == modelID {
			selectedIndex = index
		}
	}
	if selectedIndex < 0 {
		return modelresult.ConfigMutation{}, fmt.Errorf("model not found: %s", modelID)
	}
	if !configs[selectedIndex].Enabled {
		return modelresult.ConfigMutation{}, fmt.Errorf("cannot set disabled model as default: %s", modelID)
	}
	now := s.now()
	for index := range configs {
		configs[index].IsDefault = configs[index].ID == modelID
		configs[index].UpdatedAt = now
		if err := s.Repository.SaveConfig(ctx, configs[index]); err != nil {
			return modelresult.ConfigMutation{}, err
		}
	}
	s.syncRegistryMetadata(configs)
	if s.Default != nil {
		s.Default.SetDefaultModel(modelID)
	}
	return modelresult.ConfigMutation{Config: configs[selectedIndex], Message: fmt.Sprintf("Model %s is now the default model.", modelID)}, nil
}

func (s ConfigService) DeleteConfig(ctx context.Context, command modelcommand.DeleteConfig) (modelresult.ConfigMutation, error) {
	if s.Repository == nil || s.Registry == nil {
		return modelresult.ConfigMutation{}, errors.New("model config dependencies are nil")
	}
	modelID := strings.TrimSpace(command.ID)
	config, err := s.Repository.GetConfig(ctx, modelID)
	if err != nil {
		return modelresult.ConfigMutation{}, err
	}
	if config.IsDefault {
		return modelresult.ConfigMutation{}, errors.New("cannot delete the default model; select another default model first")
	}
	inUse, err := s.isModelInUse(ctx, modelID)
	if err != nil {
		return modelresult.ConfigMutation{}, err
	}
	if inUse {
		return modelresult.ConfigMutation{}, fmt.Errorf("cannot delete model %s because it is used by a session", modelID)
	}
	if err := s.Repository.DeleteConfig(ctx, modelID); err != nil {
		return modelresult.ConfigMutation{}, err
	}
	s.Registry.RemoveModel(modelID)
	return modelresult.ConfigMutation{DeletedID: modelID, Message: fmt.Sprintf("Model %s deleted.", modelID)}, nil
}

// TestConfig only creates a transient model and performs a minimal request.
// It intentionally does not persist credentials or mutate the runtime registry.
func (s ConfigService) TestConfig(ctx context.Context, command modelcommand.AddConfig) (modelresult.TestConfig, error) {
	if s.Factory == nil {
		return modelresult.TestConfig{}, errors.New("model factory is nil")
	}

	config := normalizeModelConfig(domainmodel.Config{
		ID:                        command.ID,
		Name:                      command.Name,
		Provider:                  command.Provider,
		Protocol:                  command.Protocol,
		AuthType:                  command.AuthType,
		BaseURL:                   command.BaseURL,
		APIKey:                    command.APIKey,
		ModelName:                 command.ModelName,
		DefaultGenerationSettings: generation.Clone(command.DefaultGenerationSettings),
	})
	config, err := s.prepareNewConfig(config)
	if err != nil {
		return modelresult.TestConfig{}, err
	}

	model, err := s.Factory.CreateModel(modelport.CreationConfig{
		Provider:  config.Provider,
		Protocol:  config.Protocol,
		AuthType:  config.AuthType,
		APIKey:    config.APIKey,
		BaseURL:   config.BaseURL,
		ModelName: config.ModelName,
	})
	if err != nil {
		return modelresult.TestConfig{}, err
	}
	if model == nil {
		return modelresult.TestConfig{}, errors.New("model factory returned nil model")
	}

	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	startedAt := time.Now()
	result, err := model.Generate(testCtx, modelport.GenerateRequest{
		Messages: []domainmessage.Message{
			domainmessage.Text(domainmessage.RoleUser, "Reply with OK."),
		},
		Settings: generation.SystemDefaults(),
	})
	_ = result
	if err != nil {
		return modelresult.TestConfig{}, err
	}

	return modelresult.TestConfig{
		Success:   true,
		LatencyMS: time.Since(startedAt).Milliseconds(),
		Message:   "Model connection test succeeded.",
	}, nil
}

func (s ConfigService) prepareNewConfig(config domainmodel.Config) (domainmodel.Config, error) {
	config = normalizeModelConfig(config)
	if config.Name == "" {
		config.Name = config.ID
	}
	if err := validateModelConfig(config, s.Factory); err != nil {
		return domainmodel.Config{}, err
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

func (s ConfigService) prepareUpdatedConfig(config, old domainmodel.Config) (domainmodel.Config, error) {
	if config.Name == "" {
		config.Name = old.Name
	}
	if err := validateModelConfig(config, s.Factory); err != nil {
		return domainmodel.Config{}, err
	}
	if config.ModelName == "" {
		config.ModelName = old.ModelName
		if config.ModelName == "" {
			config.ModelName = config.ID
		}
	}
	config.CreatedAt = old.CreatedAt
	config.UpdatedAt = s.now()
	config.Enabled = old.Enabled
	config.IsDefault = old.IsDefault
	return config, nil
}

func validateModelConfig(config domainmodel.Config, factory modelport.Factory) error {
	if err := generation.Validate(config.DefaultGenerationSettings); err != nil {
		return fmt.Errorf("invalid model generation settings: %w", err)
	}
	if err := validateModelID(config.ID); err != nil {
		return err
	}
	if config.Name == "" {
		return errors.New("model name is empty")
	}
	if !factory.SupportsProtocol(config.Protocol) {
		return fmt.Errorf("unsupported model protocol: %s", config.Protocol)
	}
	return factory.ValidateConfig(modelport.CreationConfig{
		Provider: config.Provider, Protocol: config.Protocol, AuthType: config.AuthType,
		APIKey: config.APIKey, BaseURL: config.BaseURL, ModelName: config.ModelName,
	})
}

func (s ConfigService) validateDependencies() error {
	if s.Repository == nil {
		return errors.New("model store is nil")
	}
	if s.Registry == nil {
		return errors.New("llm client is nil")
	}
	if s.Factory == nil {
		return errors.New("model factory is nil")
	}
	return nil
}

func (s ConfigService) createModel(config domainmodel.Config) (modelport.ChatModelPort, error) {
	return s.Factory.CreateModel(modelport.CreationConfig{
		Provider: config.Provider, Protocol: config.Protocol, AuthType: config.AuthType,
		APIKey: config.APIKey, BaseURL: config.BaseURL, ModelName: config.ModelName,
	})
}

func (s ConfigService) isModelInUse(ctx context.Context, modelID string) (bool, error) {
	if s.Usage == nil {
		return false, nil
	}
	return s.Usage.IsModelInUse(ctx, modelID)
}

func (s ConfigService) syncRegistryMetadata(configs []domainmodel.Config) {
	for _, config := range configs {
		model := s.Registry.GetModel(config.ID)
		if !config.Enabled {
			model = nil
		}
		s.Registry.SetModelInfo(config.ID, model, modelInfo(config))
	}
}

func modelInfo(config domainmodel.Config) modelport.ModelInfo {
	return modelport.ModelInfo{
		ID: config.ID, Name: config.Name, Provider: config.Provider, Protocol: config.Protocol,
		AuthType: config.AuthType, BaseURL: config.BaseURL, HasAPIKey: config.APIKey != "",
		ModelName: config.ModelName, Enabled: config.Enabled, IsDefault: config.IsDefault,
		DefaultGenerationSettings: generation.Clone(config.DefaultGenerationSettings),
	}
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func normalizeModelConfig(config domainmodel.Config) domainmodel.Config {
	config.ID = strings.TrimSpace(config.ID)
	config.Name = strings.TrimSpace(config.Name)
	config.Provider = strings.TrimSpace(strings.ToLower(config.Provider))
	if config.Provider == "" {
		config.Provider = "openai"
	}
	config.Protocol = domainmodel.NormalizeProtocol(config.Protocol)
	config.AuthType = domainmodel.NormalizeAuthType(config.AuthType)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
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
