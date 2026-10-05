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

func TestManagerReloadKeepsPreviousRuntimeWhenRequiredCandidateFails(t *testing.T) {
	t.Setenv("MYAI_MCP_TEST_HELPER", "1")
	registry := tool.NewRegisterTools()
	manager := NewManager(Config{Servers: []ServerConfig{{
		Name: "first", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, TimeoutSeconds: 5,
	}}})
	if err := manager.RegisterAll(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.GetTool("mcp_first_echo"); err != nil {
		t.Fatalf("expected initial tool: %v", err)
	}

	err := manager.Reload(context.Background(), Config{Servers: []ServerConfig{
		{Name: "first", Command: filepath.Join(t.TempDir(), "missing-mcp-server"), Required: true, TimeoutSeconds: 1},
	}}, registry)
	if err == nil {
		t.Fatal("expected reload failure")
	}
	if _, err := registry.GetTool("mcp_first_echo"); err != nil {
		t.Fatalf("previous MCP tool was removed after failed reload: %v", err)
	}
	if len(manager.clients) != 1 || len(manager.sources) != 1 {
		t.Fatalf("expected previous runtime to remain active, clients=%d sources=%d", len(manager.clients), len(manager.sources))
	}
	_ = manager.Close()
}
