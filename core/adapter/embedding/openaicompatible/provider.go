package openaicompatible

import (
	"context"
	"fmt"
	"net/http"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type embedder interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

type Provider struct {
	embedder   embedder
	dimensions int
}

var _ knowledgeport.EmbeddingProvider = (*Provider)(nil)

func New(config Config) (*Provider, error) {
	config = config.normalize()
	if err := config.validate(); err != nil {
		return nil, err
	}
	options := []openai.Option{
		openai.WithToken(config.APIKey),
		openai.WithEmbeddingModel(config.Model),
		openai.WithHTTPClient(&http.Client{Timeout: config.Timeout}),
	}
	if config.BaseURL != "" {
		options = append(options, openai.WithBaseURL(config.BaseURL))
	}
	if config.RequestDimensions {
		options = append(options, openai.WithEmbeddingDimensions(config.Dimensions))
	}
	client, err := openai.New(options...)
	if err != nil {
		return nil, fmt.Errorf("create OpenAI-compatible embedding client: %w", err)
	}
	runtimeEmbedder, err := embeddings.NewEmbedder(
		client,
		embeddings.WithBatchSize(config.BatchSize),
		embeddings.WithStripNewLines(!config.PreserveNewLines),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenAI-compatible embedder: %w", err)
	}
	return newWithEmbedder(runtimeEmbedder, config.Dimensions)
}

func newWithEmbedder(runtimeEmbedder embedder, dimensions int) (*Provider, error) {
	if runtimeEmbedder == nil {
		return nil, fmt.Errorf("embedding client is nil")
	}
	if dimensions < 1 {
		return nil, fmt.Errorf("embedding dimensions must be positive")
	}
	return &Provider{embedder: runtimeEmbedder, dimensions: dimensions}, nil
}

func (provider *Provider) EmbedDocuments(ctx context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	if err := validateRequest(request, false); err != nil {
		return domainknowledge.EmbedResult{}, err
	}
	texts := make([]string, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		texts = append(texts, input.Text)
	}
	vectors, err := provider.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return domainknowledge.EmbedResult{}, fmt.Errorf("create document embeddings: %w", err)
	}
	return provider.result(request, vectors)
}

func (provider *Provider) EmbedQuery(ctx context.Context, request domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	if err := validateRequest(request, true); err != nil {
		return domainknowledge.EmbedResult{}, err
	}
	vector, err := provider.embedder.EmbedQuery(ctx, request.Inputs[0].Text)
	if err != nil {
		return domainknowledge.EmbedResult{}, fmt.Errorf("create query embedding: %w", err)
	}
	return provider.result(request, [][]float32{vector})
}

func (provider *Provider) result(request domainknowledge.EmbedRequest, vectors [][]float32) (domainknowledge.EmbedResult, error) {
	if len(vectors) != len(request.Inputs) {
		return domainknowledge.EmbedResult{}, fmt.Errorf("embedding API returned %d vectors for %d inputs", len(vectors), len(request.Inputs))
	}
	outputs := make([]domainknowledge.EmbedOutput, 0, len(vectors))
	for index, vector := range vectors {
		if len(vector) != provider.dimensions {
			return domainknowledge.EmbedResult{}, fmt.Errorf("embedding API returned %d dimensions for input %q, expected %d", len(vector), request.Inputs[index].ID, provider.dimensions)
		}
		outputs = append(outputs, domainknowledge.EmbedOutput{
			ID:     request.Inputs[index].ID,
			Vector: append([]float32(nil), vector...),
		})
	}
	result := domainknowledge.EmbedResult{
		EmbeddingProfileID: request.EmbeddingProfileID,
		Dimensions:         provider.dimensions,
		Outputs:            outputs,
	}
	if err := result.Validate(); err != nil {
		return domainknowledge.EmbedResult{}, fmt.Errorf("validate embedding API result: %w", err)
	}
	return result, nil
}

func validateRequest(request domainknowledge.EmbedRequest, query bool) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("validate embedding request: %w", err)
	}
	if query && len(request.Inputs) != 1 {
		return fmt.Errorf("query embedding requires exactly one input")
	}
	seen := make(map[string]struct{}, len(request.Inputs))
	for _, input := range request.Inputs {
		if _, exists := seen[input.ID]; exists {
			return fmt.Errorf("embedding input id %q is duplicated", input.ID)
		}
		seen[input.ID] = struct{}{}
	}
	return nil
}
