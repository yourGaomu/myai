package local

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	retrievalresult "myai/core/application/knowledge/retrieval/result"
	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
	domainknowledge "myai/core/domain/knowledge"
	"myai/core/session"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type fakeKnowledgeRetrievalService struct {
	command searchcommand.Search
	result  searchresult.Search
	err     error
	calls   int
}

func (service *fakeKnowledgeRetrievalService) Search(_ context.Context, command searchcommand.Search) (searchresult.Search, error) {
	service.calls++
	service.command = command
	return service.result, service.err
}

func TestKnowledgeSearchToolDefinition(t *testing.T) {
	searchTool := NewKnowledgeSearchTool(&fakeKnowledgeRetrievalService{})
	if searchTool.Name() != "knowledge_search" {
		t.Fatalf("Name() = %q", searchTool.Name())
	}
	if searchTool.Permission() != tooldef.PermissionRead {
		t.Fatalf("Permission() = %q, want %q", searchTool.Permission(), tooldef.PermissionRead)
	}

	schema, ok := searchTool.Schema().(map[string]any)
	if !ok {
		t.Fatalf("Schema() type = %T", searchTool.Schema())
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("schema must reject additional properties: %#v", schema)
	}
	required, ok := schema["required"].([]string)
	if !ok || !reflect.DeepEqual(required, []string{"query"}) {
		t.Fatalf("unexpected required fields: %#v", schema["required"])
	}
}

func TestKnowledgeSearchToolMapsCommandAndResult(t *testing.T) {
	retrieval := &fakeKnowledgeRetrievalService{
		result: searchresult.Search{
			Hits: []domainknowledge.RetrievalHit{{
				KnowledgeBaseID: "kb-1",
				DocumentID:      "document-1",
				ChunkID:         "chunk-1",
				DocumentVersion: 2,
				Text:            "Plan mode implementation",
				SourceName:      "guide.md",
				SourceLocation:  "section 3",
				Score:           0.91,
				Rank:            1,
				Channel:         domainknowledge.RetrievalChannelVector,
				Origin:          domainknowledge.RetrievalOriginRemote,
			}},
			Diagnostics: searchresult.Diagnostics{
				ResolvedKnowledgeBaseIDs: []string{"kb-1", "kb-2"},
				ProfileSearches: []searchresult.ProfileSearch{{
					IndexProfileID: "index-1", EmbeddingProfileID: "embedding-1",
					KnowledgeBaseIDs: []string{"kb-1", "kb-2"},
					Diagnostics: retrievalresult.Diagnostics{
						LocalVectorHits: 1, LocalKeywordHits: 2, RemoteVectorHits: 3,
						RemoteFallback: true, CacheFillCount: 1,
						LocalQuality: domainknowledge.LocalQualityDecision{Passed: false},
					},
				}},
				Warnings: []string{"remote fallback used"},
			},
		},
	}
	searchTool := NewKnowledgeSearchTool(retrieval)

	output, err := searchTool.Call(context.Background(), mustJSON(t, map[string]any{
		"query":              "  Plan mode  ",
		"knowledge_base_ids": []string{" kb-1 ", "kb-2"},
		"category_ids":       []string{" project ", ""},
	}))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if retrieval.calls != 1 {
		t.Fatalf("Search() calls = %d, want 1", retrieval.calls)
	}
	wantCommand := searchcommand.Search{
		Text: "Plan mode", KnowledgeBaseIDs: []string{"kb-1", "kb-2"},
		CategoryIDs: []string{"project"}, TopK: defaultKnowledgeSearchTopK, Strict: true,
	}
	if !reflect.DeepEqual(retrieval.command, wantCommand) {
		t.Fatalf("unexpected retrieval command: %#v", retrieval.command)
	}

	var result knowledgeSearchResult
	if err := json.Unmarshal([]byte(output.Content), &result); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if result.Query != "Plan mode" || result.Count != 1 || len(result.Hits) != 1 {
		t.Fatalf("unexpected result summary: %#v", result)
	}
	hit := result.Hits[0]
	if hit.ChunkID != "chunk-1" || hit.Text != "Plan mode implementation" || hit.Channel != "vector" || hit.Origin != "remote" {
		t.Fatalf("unexpected hit: %#v", hit)
	}
	if !result.Diagnostics.RemoteFallback || result.Diagnostics.CacheFillCount != 1 || len(result.Diagnostics.Warnings) != 1 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestKnowledgeSearchToolPropagatesRetrievalError(t *testing.T) {
	wantErr := errors.New("retrieval unavailable")
	retrieval := &fakeKnowledgeRetrievalService{err: wantErr}
	searchTool := NewKnowledgeSearchTool(retrieval)

	_, err := searchTool.Call(context.Background(), mustJSON(t, map[string]any{
		"query": "plan mode",
		"top_k": 5,
	}))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Call() error = %v, want %v", err, wantErr)
	}
	if retrieval.command.TopK != 5 || !retrieval.command.Strict {
		t.Fatalf("unexpected retrieval command: %#v", retrieval.command)
	}
}

func TestKnowledgeSearchToolValidatesArgumentsBeforeRetrieval(t *testing.T) {
	retrieval := &fakeKnowledgeRetrievalService{}
	searchTool := NewKnowledgeSearchTool(retrieval)

	_, err := searchTool.Call(context.Background(), mustJSON(t, map[string]any{}))
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("expected query validation error, got %v", err)
	}
	if retrieval.calls != 0 {
		t.Fatalf("Search() calls = %d, want 0", retrieval.calls)
	}
}

func TestKnowledgeSearchToolRequiresRetrievalService(t *testing.T) {
	searchTool := NewKnowledgeSearchTool(nil)
	if _, err := searchTool.Call(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func TestKnowledgeSearchToolUsesSessionScopeAsHardBoundary(t *testing.T) {
	tests := []struct {
		name     string
		settings session.RAGSettings
		wantKBs  []string
		wantCats []string
	}{
		{
			name:     "category scope removes model knowledge bases",
			settings: session.RAGSettings{Mode: session.RetrievalModeManual, CategoryIDs: []string{"category-session"}, TopK: 8},
			wantCats: []string{"category-session"},
		},
		{
			name:     "knowledge base scope removes model categories",
			settings: session.RAGSettings{Mode: session.RetrievalModeManual, KnowledgeBaseIDs: []string{"kb-session"}, TopK: 8},
			wantKBs:  []string{"kb-session"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			retrieval := &fakeKnowledgeRetrievalService{}
			searchTool := NewKnowledgeSearchTool(retrieval)
			ctx := toolruntime.WithRAGSettings(context.Background(), test.settings)

			_, err := searchTool.Call(ctx, mustJSON(t, map[string]any{
				"query":              "project architecture",
				"knowledge_base_ids": []string{"kb-model"},
				"category_ids":       []string{"category-model"},
			}))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(retrieval.command.KnowledgeBaseIDs, test.wantKBs) || !reflect.DeepEqual(retrieval.command.CategoryIDs, test.wantCats) {
				t.Fatalf("session scope was not enforced: %#v", retrieval.command)
			}
		})
	}
}

func TestKnowledgeSearchToolRejectsDisabledSession(t *testing.T) {
	retrieval := &fakeKnowledgeRetrievalService{}
	searchTool := NewKnowledgeSearchTool(retrieval)
	ctx := toolruntime.WithRAGSettings(context.Background(), session.RAGSettings{Mode: session.RetrievalModeOff, TopK: 8})

	_, err := searchTool.Call(ctx, mustJSON(t, map[string]any{"query": "project architecture"}))
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled session error, got %v", err)
	}
	if retrieval.calls != 0 {
		t.Fatalf("disabled session must not search, got %d calls", retrieval.calls)
	}
}
