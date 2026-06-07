package utills

import (
	"github.com/tmc/langchaingo/llms/openai"
	"myai/core/llm"
)

func CreateLLM(apiKey string, url string, modelName string) (*llm.Model, error) {
	model, err := openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL(url),
		openai.WithModel(modelName),
	)
	if err != nil {
		return nil, err
	}

	llmodel := llm.Model{
		LlmModel: model,
	}

	return &llmodel, nil
}
