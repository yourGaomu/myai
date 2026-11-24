package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	api "myai/core/application/imagemodel/api"
	imageresult "myai/core/application/imagemodel/result"
	domain "myai/core/domain/imagemodel"
	"myai/core/imageasset"
	port "myai/core/port/imagemodel"
)

type ConfigService struct {
	Repository port.Repository
	Registry   port.MutableRegistry
	Factory    port.Factory
	Now        func() time.Time
	mu         sync.Mutex
}

var _ api.ConfigService = (*ConfigService)(nil)
var imageModelIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func (s *ConfigService) List(ctx context.Context) ([]domain.Summary, string, error) {
	if s == nil || s.Repository == nil {
		return nil, "", errors.New("image model repository is not configured")
	}
	configs, err := s.Repository.List(ctx)
	if err != nil {
		return nil, "", err
	}
	return summaries(configs)
}
func (s *ConfigService) Add(ctx context.Context, config domain.Config) (imageresult.MutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validate(); err != nil {
		return imageresult.MutationResult{}, err
	}
	old, err := s.Repository.List(ctx)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	for _, item := range old {
		if item.ID == strings.TrimSpace(config.ID) {
			return imageresult.MutationResult{}, errors.New("image model already exists")
		}
	}
	config, err = s.normalize(config)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	if !config.Enabled && config.IsDefault {
		return imageresult.MutationResult{}, errors.New("a disabled image model cannot be the default")
	}
	next := cloneConfigs(old)
	if config.IsDefault {
		for i := range next {
			next[i].IsDefault = false
		}
	}
	next = append(next, config)
	if err := s.commit(ctx, old, next); err != nil {
		return imageresult.MutationResult{}, err
	}
	return s.result(ctx, "add", config.ID, "Image model added.")
}
func (s *ConfigService) Update(ctx context.Context, command domain.Config) (imageresult.MutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validate(); err != nil {
		return imageresult.MutationResult{}, err
	}
	old, err := s.Repository.List(ctx)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	index := -1
	for i, item := range old {
		if item.ID == strings.TrimSpace(command.ID) {
			index = i
			break
		}
	}
	if index < 0 {
		return imageresult.MutationResult{}, errors.New("image model not found")
	}
	previous := old[index]
	if command.Name == "" {
		command.Name = previous.Name
	}
	if command.Provider == "" {
		command.Provider = previous.Provider
	}
	if command.AuthType == "" {
		command.AuthType = previous.AuthType
	}
	if strings.TrimSpace(command.APIKey) == "" && requiresAPIKey(command.AuthType) {
		if normalizeURL(command.BaseURL) != normalizeURL(previous.BaseURL) {
			return imageresult.MutationResult{}, errors.New("api key must be supplied when changing base url")
		}
		command.APIKey = previous.APIKey
	}
	command.CreatedAt = previous.CreatedAt
	command.Enabled = previous.Enabled
	command.IsDefault = previous.IsDefault
	config, err := s.normalize(command)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	next := cloneConfigs(old)
	next[index] = config
	if err := s.commit(ctx, old, next); err != nil {
		return imageresult.MutationResult{}, err
	}
	return s.result(ctx, "update", config.ID, "Image model updated.")
}
func (s *ConfigService) Delete(ctx context.Context, id string) (imageresult.MutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validate(); err != nil {
		return imageresult.MutationResult{}, err
	}
	id = strings.TrimSpace(id)
	old, err := s.Repository.List(ctx)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	next := make([]domain.Config, 0, len(old))
	found := false
	for _, item := range old {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return imageresult.MutationResult{}, errors.New("image model not found")
	}
	if err := s.commit(ctx, old, next); err != nil {
		return imageresult.MutationResult{}, err
	}
	return s.result(ctx, "delete", "", "Image model deleted.")
}
func (s *ConfigService) SetEnabled(ctx context.Context, id string, enabled bool) (imageresult.MutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validate(); err != nil {
		return imageresult.MutationResult{}, err
	}
	old, err := s.Repository.List(ctx)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	next := cloneConfigs(old)
	found := false
	for i := range next {
		if next[i].ID == strings.TrimSpace(id) {
			found = true
			if !enabled && next[i].IsDefault {
				return imageresult.MutationResult{}, errors.New("cannot disable the default image model")
			}
			next[i].Enabled = enabled
			next[i].UpdatedAt = s.now()
		}
	}
	if !found {
		return imageresult.MutationResult{}, errors.New("image model not found")
	}
	if err := s.commit(ctx, old, next); err != nil {
		return imageresult.MutationResult{}, err
	}
	return s.result(ctx, "set_enabled", strings.TrimSpace(id), "Image model status updated.")
}
func (s *ConfigService) SetDefault(ctx context.Context, id string) (imageresult.MutationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validate(); err != nil {
		return imageresult.MutationResult{}, err
	}
	old, err := s.Repository.List(ctx)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	next := cloneConfigs(old)
	found := false
	id = strings.TrimSpace(id)
	for i := range next {
		next[i].IsDefault = next[i].ID == id
		if next[i].ID == id && next[i].Enabled {
			found = true
		}
		if next[i].IsDefault {
			next[i].UpdatedAt = s.now()
		}
	}
	if !found {
		return imageresult.MutationResult{}, errors.New("enabled image model not found")
	}
	if err := s.commit(ctx, old, next); err != nil {
		return imageresult.MutationResult{}, err
	}
	return s.result(ctx, "set_default", id, "Default image model updated.")
}
func (s *ConfigService) Test(ctx context.Context, config domain.Config) (imageresult.TestResult, error) {
	if err := s.validate(); err != nil {
		return imageresult.TestResult{}, err
	}
	if strings.TrimSpace(config.ID) != "" && strings.TrimSpace(config.APIKey) == "" {
		if saved, err := s.Repository.Get(ctx, strings.TrimSpace(config.ID)); err == nil {
			if config.AuthType == "" {
				config.AuthType = saved.AuthType
			}
			if requiresAPIKey(config.AuthType) {
				if normalizeURL(config.BaseURL) != normalizeURL(saved.BaseURL) {
					return imageresult.TestResult{}, errors.New("api key must be supplied when changing base url")
				}
				config.APIKey = saved.APIKey
			}
		}
	}
	config, err := s.normalize(config)
	if err != nil {
		return imageresult.TestResult{}, err
	}
	generator, err := s.Factory.Build(config)
	if err != nil {
		return imageresult.TestResult{}, err
	}
	started := s.now()
	testCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	testRequest := port.GenerateRequest{Prompt: "Create a simple small test image, square composition.", AspectRatio: "1:1", OutputFormat: "png"}
	if config.Protocol == domain.ProtocolOpenAIImages {
		testRequest.Quality = "low"
	}
	result, err := generator.Generate(testCtx, testRequest)
	if err == nil {
		info, inspectErr := imageasset.Inspect(result.Data)
		if inspectErr != nil || info.MIMEType != result.MIMEType {
			err = errors.New("image model returned an invalid test image")
		}
	}
	latency := s.now().Sub(started).Milliseconds()
	if err != nil {
		return imageresult.TestResult{OK: false, Message: "Image model test failed.", LatencyMS: latency}, err
	}
	return imageresult.TestResult{OK: true, Message: "Image model is available.", LatencyMS: latency, MIMEType: result.MIMEType}, nil
}
func (s *ConfigService) normalize(c domain.Config) (domain.Config, error) {
	c.ID = strings.TrimSpace(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	c.Provider = strings.TrimSpace(c.Provider)
	c.ModelName = strings.TrimSpace(c.ModelName)
	c.BaseURL = normalizeURL(c.BaseURL)
	c.APIKey = strings.TrimSpace(c.APIKey)
	if !imageModelIDPattern.MatchString(c.ID) {
		return c, errors.New("image model id must use letters, numbers, dot, underscore, or hyphen")
	}
	if c.ModelName == "" || len(c.ModelName) > 256 {
		return c, errors.New("image model model_name is required and must be at most 256 characters")
	}
	if len(c.Name) > 128 || len(c.Provider) > 128 {
		return c, errors.New("image model name or provider is too long")
	}
	if c.Name == "" {
		c.Name = c.ID
	}
	if c.Provider == "" {
		c.Provider = string(c.Protocol)
	}
	if c.AuthType == "" {
		c.AuthType = domain.DefaultAuthType(c.Protocol)
	}
	if requiresAPIKey(c.AuthType) && c.APIKey == "" {
		return c, errors.New("image model API key is required")
	}
	if !requiresAPIKey(c.AuthType) {
		c.APIKey = ""
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = s.now()
	}
	c.UpdatedAt = s.now()
	if _, err := s.Factory.Build(c); err != nil {
		return c, err
	}
	return c, nil
}
func (s *ConfigService) commit(ctx context.Context, old, next []domain.Config) error {
	if _, _, err := summaries(next); err != nil {
		return err
	}
	for _, config := range next {
		if _, err := s.Factory.Build(config); err != nil {
			return err
		}
	}
	if err := s.persist(ctx, old, next); err != nil {
		return err
	}
	if err := s.Registry.Replace(next, s.Factory); err != nil {
		return errors.Join(err, s.rollbackPersistence(ctx, old, next))
	}
	return nil
}
func (s *ConfigService) persist(ctx context.Context, old, next []domain.Config) error {
	for _, config := range next {
		if err := s.Repository.Save(ctx, config); err != nil {
			return errors.Join(fmt.Errorf("save image model config: %w", err), s.rollbackPersistence(ctx, old, next))
		}
	}
	nextIDs := make(map[string]bool, len(next))
	for _, config := range next {
		nextIDs[config.ID] = true
	}
	for _, config := range old {
		if !nextIDs[config.ID] {
			if err := s.Repository.Delete(ctx, config.ID); err != nil {
				return errors.Join(fmt.Errorf("delete image model config: %w", err), s.rollbackPersistence(ctx, old, next))
			}
		}
	}
	return nil
}
func (s *ConfigService) rollbackPersistence(ctx context.Context, old, next []domain.Config) error {
	var errs []error
	oldIDs := make(map[string]bool, len(old))
	for _, config := range old {
		oldIDs[config.ID] = true
		if err := s.Repository.Save(ctx, config); err != nil {
			errs = append(errs, fmt.Errorf("restore image model config: %w", err))
		}
	}
	for _, config := range next {
		if !oldIDs[config.ID] {
			if err := s.Repository.Delete(ctx, config.ID); err != nil {
				errs = append(errs, fmt.Errorf("remove uncommitted image model config: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}
func (s *ConfigService) result(ctx context.Context, action, modelID, message string) (imageresult.MutationResult, error) {
	models, def, err := s.List(ctx)
	if err != nil {
		return imageresult.MutationResult{}, err
	}
	result := imageresult.MutationResult{Action: action, Models: models, DefaultModelID: def, Message: message}
	for _, model := range models {
		if model.ID == modelID {
			copy := model
			result.Model = &copy
			break
		}
	}
	return result, nil
}
func (s *ConfigService) validate() error {
	if s == nil || s.Repository == nil || s.Registry == nil || s.Factory == nil {
		return errors.New("image model configuration service is not configured")
	}
	return nil
}
func (s *ConfigService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func summaries(configs []domain.Config) ([]domain.Summary, string, error) {
	result := make([]domain.Summary, 0, len(configs))
	defaultID := ""
	for _, c := range configs {
		result = append(result, domain.ToSummary(c))
		if c.Enabled && c.IsDefault {
			if defaultID != "" {
				return nil, "", errors.New("multiple default image models")
			}
			defaultID = c.ID
		}
		if !c.Enabled && c.IsDefault {
			return nil, "", errors.New("disabled image model cannot be default")
		}
	}
	return result, defaultID, nil
}
func requiresAPIKey(authType domain.AuthType) bool {
	return authType != domain.AuthTypeNone && authType != domain.AuthTypeGoogleADC
}
func normalizeURL(value string) string { return strings.TrimRight(strings.TrimSpace(value), "/") }
func cloneConfigs(source []domain.Config) []domain.Config {
	return append([]domain.Config(nil), source...)
}
