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
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxStdioMessageSize = 16 * 1024 * 1024

type Client struct {
	// Client 通过子进程 stdin/stdout 实现 JSON-RPC；pending 用请求 ID 关联并发响应。
	config          ServerConfig
	timeout         time.Duration
	protocolVersion string

	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu  sync.Mutex
	stderrMu sync.Mutex
	stderr   []string

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
	return &Client{
		config:          config,
		timeout:         config.timeout(),
		protocolVersion: config.ProtocolVersion,
		pending:         make(map[int64]chan rpcResult),
		closed:          make(chan struct{}),
	}
}

func (c *Client) Start(ctx context.Context) error {
	// 启动后必须先完成 MCP initialize 握手，成功后才能 list/call tools。
	if c.config.Command == "" {
		return errors.New("mcp command is empty")
	}

	cmd := exec.Command(c.config.Command, c.config.Args...)
	if c.config.WorkingDir != "" {
		cmd.Dir = c.config.WorkingDir
	}
	cmd.Env = mcpProcessEnv(c.config.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	c.cmd = cmd
	c.stdin = stdin

	go c.readLoop(stdout)
	go c.logStderr(stderr)
	go func() {
		if err := cmd.Wait(); err != nil {
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
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}

		var result listToolsResult
		if err := c.request(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		tools = append(tools, result.Tools...)
		if strings.TrimSpace(result.NextCursor) == "" {
			return tools, nil
		}
		cursor = result.NextCursor
	}
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
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
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

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_, err = c.stdin.Write(payload)
	return err
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
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.timeout)
}

func mcpProcessEnv(extra map[string]string) []string {
	env := os.Environ()
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
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
