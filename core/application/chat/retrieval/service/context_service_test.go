package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	retrievalcommand "myai/core/application/chat/retrieval/command"
	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
	domainknowledge "myai/core/domain/knowledge"
	"myai/core/session"
)

func TestContextServiceHonorsSessionRetrievalModes(t *testing.T) {
	tests := []struct {
		name      string
		mode      session.RetrievalMode
		input     string
		triggered bool
		calls     int
	}{
		{name: "off", mode: session.RetrievalModeOff, input: "项目架构如何实现？"},
		{name: "manual", mode: session.RetrievalModeManual, input: "项目架构如何实现？"},
		{name: "auto skips greeting", mode: session.RetrievalModeAuto, input: "你好"},
		{name: "auto retrieves question", mode: session.RetrievalModeAuto, input: "项目架构如何实现？", triggered: true, calls: 1},
		{name: "always retrieves greeting", mode: session.RetrievalModeAlways, input: "你好", triggered: true, calls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			search := &contextSearchService{result: testContextSearchResult()}
			service := ContextService{Search: search, Policy: DefaultTriggerPolicy{}, Formatter: ContextFormatter{}}
			current := session.NewFromState(session.InitialState{
				ID: "session-1", Model: "model-1",
				RAGSettings: session.RAGSettings{Mode: test.mode, TopK: 8},
			})

			result, err := service.Prepare(context.Background(), retrievalcommand.Prepare{Session: current, Input: test.input})
			if err != nil {
				t.Fatal(err)
			}
			if result.Triggered != test.triggered || search.calls != test.calls {
				t.Fatalf("unexpected retrieval decision: result=%#v calls=%d", result, search.calls)
			}
		})
	}
}

func TestContextServiceForwardsSessionScopeAndBuildsPrompt(t *testing.T) {
	search := &contextSearchService{result: testContextSearchResult()}
	service := ContextService{Search: search, Policy: DefaultTriggerPolicy{}, Formatter: ContextFormatter{}}
	current := session.NewFromState(session.InitialState{
		ID: "session-1", Model: "model-1",
		RAGSettings: session.RAGSettings{
			Mode: session.RetrievalModeAlways, KnowledgeBaseIDs: []string{"kb-1"},
			CategoryIDs: []string{"category-1"}, TopK: 5,
		},
	})

	result, err := service.Prepare(context.Background(), retrievalcommand.Prepare{Session: current, Input: "Plan 模式"})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.commands) != 1 {
		t.Fatalf("expected one search command, got %d", len(search.commands))
	}
	command := search.commands[0]
	if !reflect.DeepEqual(command.KnowledgeBaseIDs, []string{"kb-1"}) || !reflect.DeepEqual(command.CategoryIDs, []string{"category-1"}) || command.TopK != 5 {
		t.Fatalf("unexpected session scope: %#v", command)
	}
	if !strings.Contains(result.Prompt, "guide.md") || !strings.Contains(result.Prompt, "Plan mode implementation") {
		t.Fatalf("unexpected RAG prompt: %q", result.Prompt)
	}
}

func TestContextServiceReturnsTriggeredStateWhenSearchFails(t *testing.T) {
	search := &contextSearchService{err: errors.New("milvus unavailable")}
	service := ContextService{Search: search, Policy: DefaultTriggerPolicy{}}
	current := session.NewFromState(session.InitialState{
		ID: "session-1", Model: "model-1",
		RAGSettings: session.RAGSettings{Mode: session.RetrievalModeAlways, TopK: 8},
	})

	result, err := service.Prepare(context.Background(), retrievalcommand.Prepare{Session: current, Input: "项目架构"})
	if err == nil || !result.Triggered {
		t.Fatalf("expected triggered retrieval error, got result=%#v err=%v", result, err)
	}
}

func testContextSearchResult() searchresult.Search {
	return searchresult.Search{Hits: []domainknowledge.RetrievalHit{{
		KnowledgeBaseID: "kb-1", DocumentID: "document-1", ChunkID: "chunk-1",
		Text: "Plan mode implementation", SourceName: "guide.md", Rank: 1,
	}}}
}

type contextSearchService struct {
	result   searchresult.Search
	err      error
	calls    int
	commands []searchcommand.Search
}

func (service *contextSearchService) Search(_ context.Context, command searchcommand.Search) (searchresult.Search, error) {
	service.calls++
	service.commands = append(service.commands, command)
	return service.result, service.err
}
