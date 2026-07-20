package core

import (
	"context"
	"testing"

	searchcommand "myai/core/application/knowledge/search/command"
	searchresult "myai/core/application/knowledge/search/result"
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
