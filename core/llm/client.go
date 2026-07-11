package llm

import (
	"sort"

	modelport "myai/core/port/model"
)

type ModelInfo = modelport.ModelInfo

type Client struct {
	models map[string]modelport.ChatModelPort
	infos  map[string]ModelInfo
}

var _ modelport.MutableRegistry = (*Client)(nil)

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
	info.Enabled = true

	c.models[modelName] = model
	c.infos[modelName] = info
}

func (c *Client) GetModel(name string) modelport.ChatModelPort {
	if c.models == nil {
		return nil
	}
	model, exists := c.models[name]
	if !exists {
		return nil
	}
	return model
}

func (c *Client) HasModel(name string) bool {
	return c.GetModel(name) != nil
}

func (c *Client) ListModels() []ModelInfo {
	if c.infos == nil {
		return nil
	}

	models := make([]ModelInfo, 0, len(c.infos))
	for _, info := range c.infos {
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
