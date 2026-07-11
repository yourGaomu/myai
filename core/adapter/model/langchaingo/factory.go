package langchaingo

import (
	"github.com/tmc/langchaingo/llms/openai"

	corellm "myai/core/llm"
	modelport "myai/core/port/model"
)

type Factory struct{}

func (Factory) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	model, err := openai.New(
		openai.WithToken(config.APIKey),
		openai.WithBaseURL(config.BaseURL),
		openai.WithModel(config.ModelName),
	)
	if err != nil {
		return nil, err
	}

	return &corellm.Model{LlmModel: model}, nil
}
