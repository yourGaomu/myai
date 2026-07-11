package langchaingo

import (
	"github.com/tmc/langchaingo/llms"

	modelport "myai/core/port/model"
)

func ToLLMTools(tools []modelport.Tool) []llms.Tool {
	if len(tools) == 0 {
		return nil
	}
	mapped := make([]llms.Tool, 0, len(tools))
	for _, tool := range tools {
		mapped = append(mapped, ToLLMTool(tool))
	}
	return mapped
}

func ToLLMTool(tool modelport.Tool) llms.Tool {
	mapped := llms.Tool{
		Type: tool.Type,
	}
	if mapped.Type == "" {
		mapped.Type = "function"
	}
	if tool.Function != nil {
		mapped.Function = &llms.FunctionDefinition{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		}
	}
	return mapped
}
