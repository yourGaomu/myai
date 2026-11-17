package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"myai/core/processpolicy"
	"myai/core/trust"
)

const (
	maxStdioMessageSize = 16 * 1024 * 1024
	maxToolListPages    = 100
)

type Client struct {
	// 1. stdio 使用 pending 关联响应；HTTP 在同一请求中完成 JSON-RPC 往返。
	config          ServerConfig
	timeout         time.Duration
	protocolVersion string

	cmd           *exec.Cmd
	process       *processpolicy.Process
	processClosed chan struct{}
	stdin         io.WriteCloser
	httpClient    *http.Client
	httpURL       string
	httpHeaders   http.Header
	httpCancel    context.CancelFunc
	httpContext   context.Context
	httpSessionMu sync.Mutex
	httpSessionID string

	writeGate chan struct{}
	stderrMu  sync.Mutex
	stderr    []string

	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[int64]chan rpcResult

	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

type ToolInfo struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type CallResult struct {
	Content           []ContentItem   `json:"content,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

type ContentItem struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     string          `json:"data,omitempty"`
	MIMEType string          `json:"mimeType,omitempty"`
	Resource json.RawMessage `json:"resource,omitempty"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion,omitempty"`
}

type listToolsResult struct {
	Tools      []ToolInfo `json:"tools"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcResult struct {
	response rpcResponse
	err      error
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (err *rpcError) Error() string {
	if err == nil {
		return ""
	}
	if len(err.Data) == 0 {
		return fmt.Sprintf("json-rpc error %d: %s", err.Code, err.Message)
	}
	return fmt.Sprintf("json-rpc error %d: %s: %s", err.Code, err.Message, string(err.Data))
}

func NewClient(config ServerConfig) *Client {
	client := &Client{
		config:          config,
		timeout:         config.timeout(),
		protocolVersion: config.ProtocolVersion,
		pending:         make(map[int64]chan rpcResult),
		closed:          make(chan struct{}),
		writeGate:       make(chan struct{}, 1),
	}
	client.writeGate <- struct{}{}
	return client
}

func (c *Client) Start(ctx context.Context) error {
	// 1. HTTP 服务由外部部署，本地只握手，不启动或托管短链后端进程。
	if strings.TrimSpace(c.config.URL) != "" {
		return c.startHTTP(ctx)
	}
	// 启动后必须先完成 MCP initialize 握手，成功后才能 list/call tools。
	if c.config.Command == "" {
		return errors.New("mcp command is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.config = NormalizeConfig(Config{Servers: []ServerConfig{c.config}}, c.config.TrustWorkspace).Servers[0]
	declaration, err := TrustRequest(c.config, c.config.TrustWorkspace)
	if err != nil {
		return fmt.Errorf("prepare mcp execution trust: %w", err)
	}
	if err := c.config.Trust.Authorize(ctx, declaration); err != nil {
		return fmt.Errorf("authorize mcp stdio execution: %w", err)
	}
	explicitEnv, err := expandProcessEnv(c.config.Env, os.LookupEnv)
	if err != nil {
		return err
	}

	cmd := exec.Command(c.config.Command, c.config.Args...)
	if c.config.WorkingDir != "" {
		cmd.Dir = c.config.WorkingDir
	}
	cmd.Env = mcpProcessEnv(explicitEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return err
	}

	process, err := processpolicy.Start(cmd)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return err
	}

	c.cmd = cmd
	c.process = process
	c.processClosed = make(chan struct{})
	c.stdin = stdin

	go c.readLoop(stdout)
	go c.logStderr(stderr)
	go func() {
		defer close(c.processClosed)
		if err := process.Wait(); err != nil {
			c.markClosed(err)
			return
		}
		c.markClosed(io.EOF)
	}()

	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return err
	}
	return nil
}

func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	tools := make([]ToolInfo, 0)
	cursor := ""
	seenCursors := make(map[string]struct{})
	for page := 0; page < maxToolListPages; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		var result listToolsResult
		if err := c.request(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		tools = append(tools, result.Tools...)
		nextCursor := strings.TrimSpace(result.NextCursor)
		if nextCursor == "" {
			return tools, nil
		}
		if _, exists := seenCursors[nextCursor]; exists {
			return nil, fmt.Errorf("mcp tools/list returned repeated cursor %q", nextCursor)
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	return nil, fmt.Errorf("mcp tools/list exceeded %d pages", maxToolListPages)
}

func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	args, err := decodeArguments(arguments)
	if err != nil {
		return CallResult{}, err
	}

	params := map[string]any{
		"name":      name,
		"arguments": args,
	}

	var result CallResult
	if err := c.request(ctx, "tools/call", params, &result); err != nil {
		return CallResult{}, err
	}
	return result, nil
}

func (c *Client) Close() error {
	// 1. 关闭在途 HTTP 请求和空闲连接，热更新后旧客户端不再发送请求。
	if c.httpCancel != nil {
		c.httpCancel()
	}
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.process != nil {
		_ = c.process.Kill()
		_ = c.process.Wait()
		<-c.processClosed
	}
	c.markClosed(errors.New("mcp client closed"))
	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": c.protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "myai",
			"version": "0.1.0",
		},
	}

	var result initializeResult
	if err := c.request(ctx, "initialize", params, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ProtocolVersion) != "" {
		c.protocolVersion = strings.TrimSpace(result.ProtocolVersion)
	}

	return c.notify(ctx, "notifications/initialized", map[string]any{})
}

func (c *Client) request(ctx context.Context, method string, params any, out any) error {
	// 每个请求注册一次性响应通道；超时、进程退出和正常响应都会清理 pending。
	ctx, cancel := c.contextWithTimeout(ctx)
	defer cancel()

	id := c.nextID.Add(1)
	if c.httpClient != nil {
		response, err := c.sendHTTP(ctx, rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
		if err != nil {
			return err
		}
		if response.Error != nil {
			return response.Error
		}
		if out != nil {
			if err := json.Unmarshal(response.Result, out); err != nil {
				return fmt.Errorf("decode mcp response for %s failed: %w", method, err)
			}
		}
		return nil
	}
	ch := make(chan rpcResult, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.writeJSON(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}); err != nil {
		c.removePending(id)
		return err
	}

	select {
	case result := <-ch:
		if result.err != nil {
			return result.err
		}
		if result.response.Error != nil {
			return result.response.Error
		}
		if out == nil || len(result.response.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(result.response.Result, out); err != nil {
			return fmt.Errorf("decode mcp response for %s failed: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.removePending(id)
		return c.withStderr(fmt.Errorf("mcp request %s failed: %w", method, ctx.Err()))
	case <-c.closed:
		c.removePending(id)
		if c.closeErr != nil {
			return c.withStderr(c.closeErr)
		}
		return c.withStderr(errors.New("mcp client closed"))
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	ctx, cancel := c.contextWithTimeout(ctx)
	defer cancel()

	return c.writeJSON(ctx, rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (c *Client) writeJSON(ctx context.Context, value any) error {
	if c.httpClient != nil {
		request, ok := value.(rpcRequest)
		if !ok {
			return errors.New("invalid HTTP MCP request")
		}
		_, err := c.sendHTTP(ctx, request)
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		if c.closeErr != nil {
			return c.closeErr
		}
		return errors.New("mcp client closed")
	default:
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		if c.closeErr != nil {
			return c.closeErr
		}
		return errors.New("mcp client closed")
	case <-c.writeGate:
	}
	defer func() { c.writeGate <- struct{}{} }()

	if c.stdin == nil {
		return errors.New("mcp stdin is not initialized")
	}
	writeResult := make(chan error, 1)
	go func() {
		_, writeErr := c.stdin.Write(payload)
		writeResult <- writeErr
	}()
	select {
	case err := <-writeResult:
		return err
	case <-ctx.Done():
		_ = c.stdin.Close()
		c.markClosed(ctx.Err())
		_ = c.Close()
		return ctx.Err()
	case <-c.closed:
		if c.closeErr != nil {
			return c.closeErr
		}
		return errors.New("mcp client closed")
	}
}

func (c *Client) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStdioMessageSize)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var response rpcResponse
		if err := json.Unmarshal(line, &response); err != nil {
			log.Printf("mcp %s ignored invalid stdout json: %v", c.config.Name, err)
			continue
		}
		if len(response.ID) == 0 {
			continue
		}

		id, err := parseResponseID(response.ID)
		if err != nil {
			log.Printf("mcp %s ignored response with unsupported id %s", c.config.Name, string(response.ID))
			continue
		}

		c.pendingMu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.pendingMu.Unlock()
		if ch != nil {
			ch <- rpcResult{response: response}
		}
	}

	if err := scanner.Err(); err != nil {
		c.markClosed(err)
		return
	}
	c.markClosed(io.EOF)
}

func (c *Client) logStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.appendStderr(line)
		log.Printf("mcp %s stderr: %s", c.config.Name, truncateLogLine(line, 1200))
	}
}

func (c *Client) appendStderr(line string) {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()

	const maxLines = 20
	c.stderr = append(c.stderr, line)
	if len(c.stderr) > maxLines {
		c.stderr = c.stderr[len(c.stderr)-maxLines:]
	}
}

func (c *Client) stderrTail() string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()

	if len(c.stderr) == 0 {
		return ""
	}
	return strings.Join(c.stderr, "\n")
}

func (c *Client) withStderr(err error) error {
	if err == nil {
		return nil
	}
	tail := strings.TrimSpace(c.stderrTail())
	if tail == "" {
		return err
	}
	return fmt.Errorf("%w\nmcp stderr:\n%s", err, tail)
}

func (c *Client) markClosed(err error) {
	c.closeOnce.Do(func() {
		c.closeErr = err
		close(c.closed)
		c.failPending(err)
	})
}

func (c *Client) failPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- rpcResult{err: err}
	}
}

func (c *Client) removePending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *Client) contextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, c.timeout)
}

// TrustRequest returns the same normalized stdio declaration used at startup and
// by the trust CLI. Env remains a template: approvals never depend on resolved
// secret values, and the caller must authorize before expanding those templates.
func TrustRequest(config ServerConfig, workspace string) (trust.Request, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = config.TrustWorkspace
	}
	if strings.TrimSpace(workspace) == "" {
		return trust.Request{}, errors.New("mcp trust workspace is required")
	}
	normalized := NormalizeConfig(Config{Servers: []ServerConfig{config}}, workspace).Servers[0]
	declaration := struct {
		Name            string            `json:"name"`
		URL             string            `json:"url"`
		Headers         map[string]string `json:"headers"`
		Command         string            `json:"command"`
		Args            []string          `json:"args"`
		Env             map[string]string `json:"env"`
		WorkingDir      string            `json:"working_dir"`
		Permission      string            `json:"permission"`
		TimeoutSeconds  int               `json:"timeout_seconds"`
		ProtocolVersion string            `json:"protocol_version"`
		Disabled        bool              `json:"disabled"`
		Required        bool              `json:"required"`
	}{normalized.Name, normalized.URL, normalized.Headers, normalized.Command, normalized.Args,
		normalized.Env, normalized.WorkingDir, normalized.Permission, normalized.TimeoutSeconds,
		normalized.ProtocolVersion, normalized.Disabled, normalized.Required}
	return trust.NewRequest(trust.KindMCP, workspace, workspace, declaration)
}

// Expansion is deliberately limited to ${NAME} references in explicitly declared
// values. A missing reference fails startup without including environment values.
func expandProcessEnv(extra map[string]string, lookup func(string) (string, bool)) (map[string]string, error) {
	expanded := make(map[string]string, len(extra))
	for name, template := range extra {
		var value strings.Builder
		for {
			start := strings.Index(template, "${")
			if start < 0 {
				value.WriteString(template)
				break
			}
			value.WriteString(template[:start])
			template = template[start+2:]
			end := strings.IndexByte(template, '}')
			if end < 0 {
				return nil, errors.New("invalid mcp environment reference")
			}
			reference := template[:end]
			if !validEnvironmentReference(reference) {
				return nil, errors.New("invalid mcp environment reference")
			}
			replacement, ok := lookup(reference)
			if !ok {
				return nil, errors.New("mcp environment reference is not set")
			}
			value.WriteString(replacement)
			template = template[end+1:]
		}
		expanded[name] = value.String()
	}
	return expanded, nil
}

func validEnvironmentReference(name string) bool {
	if name == "" {
		return false
	}
	for index, letter := range name {
		if letter == '_' || letter >= 'a' && letter <= 'z' || letter >= 'A' && letter <= 'Z' || index > 0 && letter >= '0' && letter <= '9' {
			continue
		}
		return false
	}
	return true
}

func mcpProcessEnv(extra map[string]string) []string {
	return processpolicy.Environment(extra)
}

func parseResponseID(raw json.RawMessage) (int64, error) {
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}
	return strconv.ParseInt(text, 10, 64)
}

func decodeArguments(raw json.RawMessage) (any, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var args any
	if err := decoder.Decode(&args); err != nil {
		return nil, fmt.Errorf("decode tool arguments failed: %w", err)
	}
	if args == nil {
		return map[string]any{}, nil
	}
	return args, nil
}

func truncateLogLine(text string, maxLength int) string {
	runes := []rune(text)
	if len(runes) <= maxLength {
		return text
	}
	return string(runes[:maxLength]) + "..."
}
