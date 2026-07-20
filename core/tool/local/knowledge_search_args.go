package local

import (
	"encoding/json"
	"errors"
	"strings"

	searchcommand "myai/core/application/knowledge/search/command"
)

const defaultKnowledgeSearchTopK = 8

type knowledgeSearchArgs struct {
	Query            string   `json:"query"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
	CategoryIDs      []string `json:"category_ids"`
	TopK             int      `json:"top_k"`
}

func decodeKnowledgeSearchCommand(args json.RawMessage) (searchcommand.Search, error) {
	var input knowledgeSearchArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return searchcommand.Search{}, err
		}
	}

	input.Query = strings.TrimSpace(input.Query)
	for index := range input.KnowledgeBaseIDs {
		input.KnowledgeBaseIDs[index] = strings.TrimSpace(input.KnowledgeBaseIDs[index])
	}
	for index := range input.CategoryIDs {
		input.CategoryIDs[index] = strings.TrimSpace(input.CategoryIDs[index])
	}
	if input.TopK == 0 {
		input.TopK = defaultKnowledgeSearchTopK
	}

	if input.Query == "" {
		return searchcommand.Search{}, errors.New("knowledge search query is required")
	}
	if input.TopK < 1 {
		return searchcommand.Search{}, errors.New("knowledge search top_k must be positive")
	}
	return searchcommand.Search{
		Text: input.Query, KnowledgeBaseIDs: compactKnowledgeSearchIDs(input.KnowledgeBaseIDs),
		CategoryIDs: compactKnowledgeSearchIDs(input.CategoryIDs), TopK: input.TopK,
	}, nil
}

func compactKnowledgeSearchIDs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
