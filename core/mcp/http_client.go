package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var httpEnvironmentVariable = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func (c *Client) startHTTP(ctx context.Context) error {
	// 1. 远程地址与进程命令互斥，避免配置错误时启动意外进程。
	if strings.TrimSpace(c.config.Command) != "" {
		return errors.New("mcp server must configure either url or command, not both")
	}
	endpoint, err := expandHTTPEnvironment(c.config.URL)
	if err != nil {
		return err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("mcp HTTP url must be an absolute http(s) URL without userinfo or fragment")
	}
	// 1.1 凭据从环境变量读取，禁止写入工具描述，也不把服务端错误正文写进日志。
	headers := make(http.Header)
	for name, template := range c.config.Headers {
		value, err := expandHTTPEnvironment(template)
		if err != nil {
			return err
		}
		if !validHTTPHeaderName(name) || strings.ContainsAny(value, "\r\n") {
			return errors.New("invalid mcp HTTP header")
		}
		switch strings.ToLower(name) {
		case "host", "content-length", "content-type", "accept", "mcp-session-id", "mcp-protocol-version":
			return fmt.Errorf("mcp HTTP header %s is managed by the transport", name)
		}
		headers.Set(name, value)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	c.httpClient = &http.Client{
		Transport: transport,
		// 1.2 不跟随重定向，不自动重试 tools/call，避免泄漏凭据和重复创建短链。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	c.httpURL = endpoint
	c.httpHeaders = headers
	c.httpContext, c.httpCancel = context.WithCancel(context.Background())
	if c.protocolVersion == "" {
		c.protocolVersion = defaultProtocolVersion
	}
	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return err
	}
	return nil
}

func (c *Client) sendHTTP(ctx context.Context, message rpcRequest) (*rpcResponse, error) {
	// 2. 每次 POST 独立关联 JSON-RPC ID；关闭客户端同时取消在途请求。
	select {
	case <-c.closed:
		return nil, errors.New("mcp client closed")
	default:
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.httpContext, cancel)
	defer stop()
	defer cancel()
	body, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.httpURL, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid mcp HTTP request")
	}
	request.Header = c.httpHeaders.Clone()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if message.Method != "initialize" {
		request.Header.Set("MCP-Protocol-Version", c.protocolVersion)
		c.httpSessionMu.Lock()
		sessionID := c.httpSessionID
		c.httpSessionMu.Unlock()
		if sessionID != "" {
			request.Header.Set("Mcp-Session-Id", sessionID)
		}
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		// 2.1 url.Error 含完整 URL；向上仅返回内部原因，防止查询参数中的凭据泄漏。
		var transportError *url.Error
		if errors.As(err, &transportError) {
			err = transportError.Err
		}
		return nil, fmt.Errorf("mcp HTTP %s failed: %w", message.Method, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp HTTP %s returned status %d", message.Method, response.StatusCode)
	}
	if message.ID == 0 {
		return nil, nil
	}
	if response.StatusCode == http.StatusAccepted || response.StatusCode == http.StatusNoContent {
		return nil, fmt.Errorf("mcp HTTP %s returned no JSON-RPC response", message.Method)
	}
	// 2.2 初始化可返回会话标识；无状态短链服务不返回该标识也能正常调用。
	if message.Method == "initialize" {
		if sessionID := response.Header.Get("Mcp-Session-Id"); sessionID != "" {
			for _, character := range sessionID {
				if character < 0x21 || character > 0x7e {
					return nil, errors.New("invalid mcp HTTP session ID")
				}
			}
			c.httpSessionMu.Lock()
			c.httpSessionID = sessionID
			c.httpSessionMu.Unlock()
		}
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return nil, errors.New("invalid mcp HTTP response content type")
	}
	limited := &io.LimitedReader{R: response.Body, N: maxStdioMessageSize + 1}
	if contentType == "text/event-stream" {
		return readHTTPEventResponse(limited, message.ID)
	}
	if contentType != "application/json" {
		return nil, errors.New("unsupported mcp HTTP response content type")
	}
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(payload) > maxStdioMessageSize {
		return nil, errors.New("mcp HTTP response exceeds size limit")
	}
	return decodeHTTPResponse(payload, message.ID)
}

func readHTTPEventResponse(reader *io.LimitedReader, requestID int64) (*rpcResponse, error) {
	// 3. 同时支持 JSON 和 POST SSE 响应，只消费当前请求的最终响应。
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxStdioMessageSize)
	var data []string
	for scanner.Scan() {
		if reader.N == 0 {
			return nil, errors.New("mcp HTTP event stream exceeds size limit")
		}
		line := scanner.Text()
		if line == "" {
			if len(data) == 0 {
				continue
			}
			payload := []byte(strings.Join(data, "\n"))
			data = nil
			var envelope rpcResponse
			if err := json.Unmarshal(payload, &envelope); err != nil {
				return nil, errors.New("invalid mcp HTTP event JSON")
			}
			// 3.1 服务端通知没有响应 ID，不得误认为 tools/call 的返回。
			if len(envelope.ID) == 0 || (len(envelope.Result) == 0 && envelope.Error == nil) {
				continue
			}
			return decodeHTTPResponse(payload, requestID)
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(value, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read mcp HTTP event stream: %w", err)
	}
	return nil, errors.New("mcp HTTP event stream ended without a response")
}

func decodeHTTPResponse(payload []byte, requestID int64) (*rpcResponse, error) {
	var response rpcResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, errors.New("invalid mcp HTTP response JSON")
	}
	id, err := parseResponseID(response.ID)
	if err != nil || id != requestID || response.JSONRPC != "2.0" {
		return nil, errors.New("mcp HTTP response ID or JSON-RPC version mismatch")
	}
	if (len(response.Result) == 0) == (response.Error == nil) {
		return nil, errors.New("mcp HTTP response must contain exactly one of result or error")
	}
	return &response, nil
}

func expandHTTPEnvironment(template string) (string, error) {
	// 4. 只展开显式 ${NAME}，变量缺失时拒绝连接，不发送空凭据。
	var missing string
	value := httpEnvironmentVariable.ReplaceAllStringFunc(template, func(match string) string {
		name := match[2 : len(match)-1]
		value, ok := os.LookupEnv(name)
		if !ok || strings.TrimSpace(value) == "" {
			missing = name
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("mcp HTTP environment variable %s is not configured", missing)
	}
	return strings.TrimSpace(value), nil
}

func validHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, character := range name {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}
