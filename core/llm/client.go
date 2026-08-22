package llm

import (
	"sort"
	"sync"

	generation "myai/core/domain/generation"
	modelport "myai/core/port/model"
)

type ModelInfo = modelport.ModelInfo

type Client struct {
	mu     sync.RWMutex
	models map[string]modelport.ChatModelPort
	infos  map[string]ModelInfo
}

var _ modelport.MutableRegistry = (*Client)(nil)
var _ modelport.MetadataProvider = (*Client)(nil)

func (c *Client) GetModelInfo(modelID string) (ModelInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	info, ok := c.infos[modelID]
	info.DefaultGenerationSettings = generation.Clone(info.DefaultGenerationSettings)
	return info, ok
}

func NewClient() *Client {
	return &Client{
		models: make(map[string]modelport.ChatModelPort),
		infos:  make(map[string]ModelInfo),
	}
}

func (c *Client) SetModel(modelName string, model modelport.ChatModelPort) {
	c.SetModelInfo(modelName, model, ModelInfo{
		ID:        modelName,
		Name:      modelName,
		ModelName: modelName,
		Enabled:   true,
	})
}

func (c *Client) SetModelInfo(modelName string, model modelport.ChatModelPort, info ModelInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.models == nil {
		c.models = map[string]modelport.ChatModelPort{}
	}
	if c.infos == nil {
		c.infos = map[string]ModelInfo{}
	}
	if info.ID == "" {
		info.ID = modelName
	}
	if info.Name == "" {
		info.Name = info.ID
	}
	if info.ModelName == "" {
		info.ModelName = info.ID
	}
	info.DefaultGenerationSettings = generation.Clone(info.DefaultGenerationSettings)

	c.models[modelName] = model
	c.infos[modelName] = info
}

func (c *Client) RemoveModel(modelName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.models[modelName]; !ok {
		return false
	}
	delete(c.models, modelName)
	delete(c.infos, modelName)
	return true
}

func (c *Client) GetModel(name string) modelport.ChatModelPort {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.models == nil {
		return nil
	}
	model, exists := c.models[name]
	info, infoExists := c.infos[name]
	if !exists || !infoExists || !info.Enabled {
		return nil
	}
	return model
}

func (c *Client) HasModel(name string) bool {
	return c.GetModel(name) != nil
}

func (c *Client) ListModels() []ModelInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.infos == nil {
		return nil
	}

	models := make([]ModelInfo, 0, len(c.infos))
	for _, info := range c.infos {
		info.DefaultGenerationSettings = generation.Clone(info.DefaultGenerationSettings)
		models = append(models, info)
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].IsDefault != models[j].IsDefault {
			return models[i].IsDefault
		}
		return models[i].ID < models[j].ID
	})

	return models
}
