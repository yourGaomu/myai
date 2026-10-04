package core

import (
	"context"
	"testing"

	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
	memoryretrievalcommand "myai/core/application/memory/retrieval/command"
	memoryretrievalresult "myai/core/application/memory/retrieval/result"
)

type appFakeRetrievalService struct{}

func (appFakeRetrievalService) Search(context.Context, searchcommand.Search) (searchresult.Search, error) {
	return searchresult.Search{}, nil
}

func TestInitRegisterSkipsKnowledgeSearchWithoutRetrievalService(t *testing.T) {
	app := &Application{}
	registry := app.InitRegister()

	if _, err := registry.GetTool("knowledge_search"); err == nil {
		t.Fatal("knowledge_search must not be registered without RetrievalService")
	}
}

func TestInitRegisterAddsKnowledgeSearchWithRetrievalService(t *testing.T) {
	app := &Application{knowledgeSearchService: appFakeRetrievalService{}}
	registry := app.InitRegister()

	registered, err := registry.GetTool("knowledge_search")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if registered.Name() != "knowledge_search" {
		t.Fatalf("registered tool name = %q", registered.Name())
	}
}

func TestInitRegisterSkipsMemorySearchWithoutRetrievalService(t *testing.T) {
	registry := (&Application{}).InitRegister()
	if _, err := registry.GetTool("memory_search"); err == nil {
		t.Fatal("memory_search must not be registered without memory retrieval service")
	}
}

func TestInitRegisterAddsMemorySearchWithRetrievalService(t *testing.T) {
	app := &Application{memoryRetrievalService: appFakeMemoryContextPreparer{}}
	registry := app.InitRegister()
	registered, err := registry.GetTool("memory_search")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if registered.Name() != "memory_search" {
		t.Fatalf("registered tool name = %q", registered.Name())
	}
}

type appFakeMemoryContextPreparer struct{}

func (appFakeMemoryContextPreparer) Prepare(context.Context, memoryretrievalcommand.Prepare) (memoryretrievalresult.Context, error) {
	return memoryretrievalresult.Context{}, nil
}
