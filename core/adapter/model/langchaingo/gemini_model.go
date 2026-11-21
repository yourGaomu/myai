package langchaingo

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/tmc/langchaingo/llms"
	"google.golang.org/protobuf/encoding/protojson"
)

type geminiModel struct {
	name     string
	endpoint string
	client   *http.Client
}

var _ llms.Model = (*geminiModel)(nil)

func (m *geminiModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, m, prompt, options...)
}

func (m *geminiModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	opts := geminiDefaultCallOptions(m.name)
	for _, apply := range options {
		apply(&opts)
	}
	payload, err := geminiRequest(m.name, messages, opts)
	if err != nil {
		return nil, err
	}
	body, err := (protojson.MarshalOptions{AllowPartial: true, UseEnumNumbers: true}).Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Gemini request: %w", err)
	}
	// The previous SDK uses the stream endpoint for history even when the
	// caller does not request callbacks. Keep that wire behavior.
	streaming := opts.StreamingFunc != nil || len(messages) > 1
	method := ":generateContent"
	if streaming {
		method = ":streamGenerateContent"
	}
	endpoint, err := url.Parse(m.endpoint)
	if err != nil {
		return nil, err
	}
	endpoint.Path += "/v1beta/" + payload.GetModel() + method
	query := endpoint.Query()
	query.Set("$alt", "json;enum-encoding=int")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readGeminiResponse(ctx, resp.Body, streaming, opts.StreamingFunc)
}
