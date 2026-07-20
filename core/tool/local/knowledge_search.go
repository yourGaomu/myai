package local

import (
	"context"
	"encoding/json"
	"errors"

	searchapi "myai/core/application/knowledge/search/api"
	"myai/core/session"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type KnowledgeSearchTool struct {
	search searchapi.Service
}

var _ tooldef.Tool = (*KnowledgeSearchTool)(nil)

func NewKnowledgeSearchTool(search searchapi.Service) *KnowledgeSearchTool {
	return &KnowledgeSearchTool{search: search}
}

func (tool *KnowledgeSearchTool) Name() string {
	return "knowledge_search"
}

func (tool *KnowledgeSearchTool) Description() string {
	return "Search indexed project knowledge bases and return relevant text chunks, source references, and retrieval diagnostics."
}

func (tool *KnowledgeSearchTool) Schema() any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Natural-language query used for semantic and keyword retrieval.",
			},
			"knowledge_base_ids": map[string]any{
				"type":        "array",
				"description": "Knowledge base IDs to search. Omit or use an empty array to search across all knowledge bases.",
				"items": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
			},
			"category_ids": map[string]any{
				"type":        "array",
				"description": "Knowledge category IDs to search, including all descendant categories.",
				"items": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "Maximum number of relevant chunks to return. Defaults to 8.",
				"minimum":     1,
			},
		},
		"required": []string{"query"},
	}
}

func (tool *KnowledgeSearchTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (tool *KnowledgeSearchTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if tool == nil || tool.search == nil {
		return tooldef.ToolOutput{}, errors.New("knowledge search service is not configured")
	}

	command, err := decodeKnowledgeSearchCommand(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}

	if settings, ok := toolruntime.RAGSettings(ctx); ok {
		settings = session.NormalizeRAGSettings(settings)
		if settings.Mode == session.RetrievalModeOff {
			return tooldef.ToolOutput{}, errors.New("knowledge search is disabled for this session")
		}
		// Session 范围是硬边界；一旦配置，工具参数不能替换或扩大这个范围。
		if len(settings.KnowledgeBaseIDs) > 0 || len(settings.CategoryIDs) > 0 {
			command.KnowledgeBaseIDs = append([]string(nil), settings.KnowledgeBaseIDs...)
			command.CategoryIDs = append([]string(nil), settings.CategoryIDs...)
		}
	}
	command.Strict = true
	response, err := tool.search.Search(ctx, command)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}

	content, err := json.MarshalIndent(newKnowledgeSearchResult(command.Text, response), "", "  ")
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return tooldef.SuccessOutput(string(content)), nil
}
