package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestReloadMCPUsesStartupConfigPathAndIgnoresUnrelatedSettings(t *testing.T) {
	deployment := t.TempDir()
	t.Chdir(deployment)
	t.Setenv("SHORT_LINKS_MCP_URL", "")
	t.Setenv("MCP_SERVERS", "")
	t.Setenv("MYAI_API_KEY", "fixture-deployment-value")
	path := filepath.Join(deployment, "resource", "application.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	config := "mcp:\n  servers:\n    - name: startup\n      disabled: true\n"
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	app := &Application{workspace: t.TempDir()}
	app.InitConfig()
	app.InitRegister()
	app.InitMCP()
	t.Cleanup(func() { _ = app.mcpManager.Close() })

	t.Chdir(t.TempDir())
	config = strings.Replace(config, "startup", "reloaded", 1) + "rag:\n  embedding:\n    models: invalid-fixture-field\n"
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.ReloadMCP(context.Background()); err != nil {
		t.Fatal(err)
	}
	statuses, err := app.MCPServerStatuses()
	if err != nil || len(statuses) != 1 || statuses[0].Name != "reloaded" {
		t.Fatalf("unexpected MCP inventory after reload: %v, %v", statuses, err)
	}
}

type appFakeMemoryContextPreparer struct{}

func (appFakeMemoryContextPreparer) Prepare(context.Context, memoryretrievalcommand.Prepare) (memoryretrievalresult.Context, error) {
	return memoryretrievalresult.Context{}, nil
}
