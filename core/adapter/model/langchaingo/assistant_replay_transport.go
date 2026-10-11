package langchaingo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	llmmapper "myai/core/adapter/llm/langchaingo"
)

type assistantReplayTransport struct {
	base http.RoundTripper
}

func (t assistantReplayTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	replay := llmmapper.AssistantReplayData(request.Context())
	if len(replay) == 0 || request.Body == nil || !strings.HasSuffix(request.URL.Path, "/chat/completions") {
		return t.base.RoundTrip(request)
	}
	// 1. 补回 SDK 遗失的工具调用伴随正文与原始推理，不把推理混入正文。
	body, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read chat request: %w", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode chat request: %w", err)
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(payload["messages"], &messages); err != nil {
		return nil, fmt.Errorf("decode chat messages: %w", err)
	}
	index := 0
	for _, message := range messages {
		var role string
		if err := json.Unmarshal(message["role"], &role); err != nil {
			return nil, fmt.Errorf("decode chat role: %w", err)
		}
		if role != "assistant" {
			continue
		}
		if index >= len(replay) {
			return nil, fmt.Errorf("assistant replay count does not match chat messages")
		}
		message["content"], _ = json.Marshal(replay[index].Content)
		if replay[index].Reasoning != "" {
			message["reasoning_content"], _ = json.Marshal(replay[index].Reasoning)
		}
		index++
	}
	if index != len(replay) {
		return nil, fmt.Errorf("assistant replay count does not match chat messages")
	}
	payload["messages"], err = json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	body, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// 2. 克隆请求并更新长度与重试读取器，保持原有认证和取消语义。
	clone := request.Clone(request.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return t.base.RoundTrip(clone)
}
