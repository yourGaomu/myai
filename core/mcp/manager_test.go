package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"myai/core/tool"
)

func TestManagerRollsBackEarlierServersWhenRequiredServerFails(t *testing.T) {
	t.Setenv("MYAI_MCP_TEST_HELPER", "1")
	registry := tool.NewRegisterTools()
	manager := NewManager(Config{Servers: []ServerConfig{
		{Name: "first", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, TimeoutSeconds: 5},
		{Name: "required", Command: filepath.Join(t.TempDir(), "missing-mcp-server"), Required: true, TimeoutSeconds: 1},
	}})

	if err := manager.RegisterAll(context.Background(), registry); err == nil {
		t.Fatal("expected required MCP server failure")
	}
	if len(manager.clients) != 0 || len(manager.sources) != 0 {
		t.Fatalf("expected manager runtimes to be rolled back, clients=%d sources=%d", len(manager.clients), len(manager.sources))
	}
	if _, err := registry.GetTool("mcp_first_echo"); err == nil {
		t.Fatal("rolled back MCP tool must not remain registered")
	}
}

func TestManagerCloseUnregistersTools(t *testing.T) {
	t.Setenv("MYAI_MCP_TEST_HELPER", "1")
	registry := tool.NewRegisterTools()
	manager := NewManager(Config{Servers: []ServerConfig{{
		Name: "first", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, TimeoutSeconds: 5,
	}}})
	if err := manager.RegisterAll(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.GetTool("mcp_first_echo"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.GetTool("mcp_first_echo"); err == nil {
		t.Fatal("closed MCP manager tool must be unregistered")
	}
}
