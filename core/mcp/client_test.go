package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"myai/core/trust"
)

func TestClientListAndCallTool(t *testing.T) {
	client := NewClient(ServerConfig{
		Name:           "test",
		Command:        os.Args[0],
		Args:           []string{"-test.run=TestMCPHelperProcess"},
		TimeoutSeconds: 5,
		Env:            map[string]string{"MYAI_MCP_TEST_HELPER": "1"},
		Trust:          &trust.Policy{Allow: true},
		TrustWorkspace: t.TempDir(),
	})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", tools)
	}

	result, err := client.CallTool(context.Background(), "echo", json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if got := formatCallResult(result); got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}

func TestClientListToolsRejectsRepeatedCursor(t *testing.T) {
	client := NewClient(ServerConfig{
		Name: "test", Command: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess"}, TimeoutSeconds: 5,
		Env:   map[string]string{"MYAI_MCP_TEST_HELPER": "1", "MYAI_MCP_TEST_CURSOR_LOOP": "1"},
		Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(),
	})
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer client.Close()
	if _, err := client.ListTools(context.Background()); err == nil {
		t.Fatal("expected repeated tools/list cursor to fail")
	}
}

func TestClientWriteCancellationClosesBlockedStdin(t *testing.T) {
	client := NewClient(ServerConfig{Name: "blocked", TimeoutSeconds: 1})
	writer := newBlockingWriteCloser()
	client.stdin = writer
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := client.writeJSON(ctx, rpcRequest{JSONRPC: "2.0", Method: "test"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	select {
	case <-writer.started:
	default:
		t.Fatal("expected stdin write to start")
	}
	select {
	case <-writer.closed:
	default:
		t.Fatal("expected blocked stdin to be closed on cancellation")
	}
}

func trustedHelperConfig(t *testing.T, extra map[string]string) ServerConfig {
	t.Helper()
	environment := map[string]string{"MYAI_MCP_TEST_HELPER": "1"}
	for name, value := range extra {
		environment[name] = value
	}
	return ServerConfig{
		Name: "process-boundary", Command: os.Args[0], Args: []string{"-test.run=^TestMCPHelperProcess$"},
		TimeoutSeconds: 2, Env: environment, Trust: &trust.Policy{Allow: true}, TrustWorkspace: t.TempDir(),
	}
}

func TestClientProcessEnvironmentIsMinimalAndExplicitEnvWorks(t *testing.T) {
	t.Setenv("ZZ_TEST_SECRET", "dummy-service-secret")
	for _, explicit := range []bool{false, true} {
		extra := map[string]string{"ZZ_TEST_EXPECT_ENV": "absent"}
		if explicit {
			extra["ZZ_TEST_SECRET"] = "explicit-dummy-secret"
			extra["ZZ_TEST_EXPECT_ENV"] = "explicit"
		}
		client := NewClient(trustedHelperConfig(t, extra))
		if err := client.Start(context.Background()); err != nil {
			t.Fatalf("environment boundary failed (explicit=%v): %v", explicit, err)
		}
		_ = client.Close()
	}
}

func TestClientCloseTerminatesDescendantsPromptly(t *testing.T) {
	directory := t.TempDir()
	client := NewClient(trustedHelperConfig(t, map[string]string{"ZZ_TEST_TREE_DIR": directory}))
	if err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	pid := awaitMCPPID(t, filepath.Join(directory, "child.pid"))
	started := time.Now()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("MCP shutdown exceeded bound: %s", elapsed)
	}
	assertMCPStopped(t, pid)
	t.Logf("MCP Close returned in %s with descendant stopped", time.Since(started))
}

func TestClientStartupTimeoutCleansProcessTree(t *testing.T) {
	directory := t.TempDir()
	client := NewClient(trustedHelperConfig(t, map[string]string{"ZZ_TEST_TREE_DIR": directory, "ZZ_TEST_NO_READ": "1"}))
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := client.Start(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected startup deadline, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("MCP startup cleanup exceeded bound: %s", elapsed)
	}
	pid := awaitMCPPID(t, filepath.Join(directory, "child.pid"))
	assertMCPStopped(t, pid)
}

func TestClientRejectsUntrustedExecutionBeforeStarting(t *testing.T) {
	config := trustedHelperConfig(t, nil)
	config.Trust = &trust.Policy{Store: trust.NewStore(filepath.Join(t.TempDir(), "approvals.json"))}
	client := NewClient(config)
	if err := client.Start(context.Background()); !errors.Is(err, trust.ErrUntrusted) {
		t.Fatalf("expected execution trust failure, got %v", err)
	}
	if client.cmd != nil {
		t.Fatal("untrusted command was started")
	}
}

func TestTrustRequestCoversDeclarationAndIgnoresPolicyFunctions(t *testing.T) {
	config := trustedHelperConfig(t, map[string]string{"ZZ_TEST_TOKEN": "${ZZ_TEST_SECRET}"})
	config.Trust.Confirm = func(context.Context, trust.Request) (trust.Approval, error) { return trust.Approval{}, nil }
	original, err := TrustRequest(config, config.TrustWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*ServerConfig){
		"command":    func(c *ServerConfig) { c.Command += "-changed" },
		"args":       func(c *ServerConfig) { c.Args = []string{"changed"} },
		"env":        func(c *ServerConfig) { c.Env = map[string]string{"ZZ_TEST_TOKEN": "${ZZ_TEST_OTHER}"} },
		"workdir":    func(c *ServerConfig) { c.WorkingDir = "changed" },
		"permission": func(c *ServerConfig) { c.Permission = "read" },
		"timeout":    func(c *ServerConfig) { c.TimeoutSeconds++ },
		"protocol":   func(c *ServerConfig) { c.ProtocolVersion = "changed" },
		"required":   func(c *ServerConfig) { c.Required = !c.Required },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			candidate := config
			change(&candidate)
			request, err := TrustRequest(candidate, config.TrustWorkspace)
			if err != nil {
				t.Fatal(err)
			}
			if request.Digest == original.Digest {
				t.Fatal("execution declaration change did not invalidate approval")
			}
		})
	}
}

func TestExplicitProcessEnvReferencesOnlyDeclaredNames(t *testing.T) {
	reads := 0
	result, err := expandProcessEnv(map[string]string{"ZZ_TEST_TOKEN": "prefix-${ZZ_TEST_SECRET}-suffix"}, func(name string) (string, bool) {
		reads++
		if name != "ZZ_TEST_SECRET" {
			t.Fatal("read an undeclared environment reference")
		}
		return "dummy-secret", true
	})
	if err != nil || reads != 1 || result["ZZ_TEST_TOKEN"] != "prefix-dummy-secret-suffix" {
		t.Fatal("explicit environment reference was not expanded")
	}
	for _, template := range []string{"${ZZ_TEST_MISSING}", "${}", "${bad-name}", "${UNCLOSED"} {
		if _, err := expandProcessEnv(map[string]string{"ZZ_TEST_TOKEN": template}, func(string) (string, bool) { return "", false }); err == nil {
			t.Fatal("invalid or missing reference did not fail closed")
		}
	}
}

func awaitMCPPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("MCP descendant did not report PID")
	return 0
}

func TestMCPHelperProcess(t *testing.T) {
	if len(os.Args) >= 3 && os.Args[len(os.Args)-2] == "mcp-tree-leaf" {
		_ = os.WriteFile(os.Args[len(os.Args)-1], []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(15 * time.Second)
		os.Exit(0)
	}
	if os.Getenv("MYAI_MCP_TEST_HELPER") != "1" {
		return
	}
	defer os.Exit(0)
	if expected := os.Getenv("ZZ_TEST_EXPECT_ENV"); expected != "" {
		actual, present := os.LookupEnv("ZZ_TEST_SECRET")
		if expected == "absent" && present || expected == "explicit" && actual != "explicit-dummy-secret" {
			os.Exit(7)
		}
	}
	if directory := os.Getenv("ZZ_TEST_TREE_DIR"); directory != "" {
		child := exec.Command(os.Args[0], "-test.run=^TestMCPHelperProcess$", "--", "mcp-tree-leaf", filepath.Join(directory, "child.pid"))
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(8)
		}
		defer child.Wait()
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	if os.Getenv("ZZ_TEST_NO_READ") == "1" {
		time.Sleep(15 * time.Second)
		return
	}
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			continue
		}
		if len(request.ID) == 0 {
			continue
		}

		switch request.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result": map[string]any{
					"protocolVersion": defaultProtocolVersion,
					"capabilities":    map[string]any{},
					"serverInfo": map[string]string{
						"name":    "test-mcp",
						"version": "0.0.1",
					},
				},
			})
		case "tools/list":
			result := map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo a message.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]string{"type": "string"},
							},
						},
					},
				},
			}
			if os.Getenv("MYAI_MCP_TEST_CURSOR_LOOP") == "1" {
				result["nextCursor"] = "repeat"
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result":  result,
			})
		case "tools/call":
			var params struct {
				Name      string `json:"name"`
				Arguments struct {
					Message string `json:"message"`
				} `json:"arguments"`
			}
			_ = json.Unmarshal(request.Params, &params)
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(request.ID),
				"result": map[string]any{
					"content": []map[string]string{
						{"type": "text", "text": params.Arguments.Message},
					},
				},
			})
		}
	}
}

type blockingWriteCloser struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func newBlockingWriteCloser() *blockingWriteCloser {
	return &blockingWriteCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (writer *blockingWriteCloser) Write([]byte) (int, error) {
	writer.once.Do(func() { close(writer.started) })
	<-writer.closed
	return 0, errors.New("writer closed")
}

func (writer *blockingWriteCloser) Close() error {
	select {
	case <-writer.closed:
	default:
		close(writer.closed)
	}
	return nil
}
