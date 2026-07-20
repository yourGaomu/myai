package openaicompatible

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainknowledge "myai/core/domain/knowledge"
)

func TestProviderUsesOpenAICompatibleHTTPContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected embedding path %q", request.URL.Path)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "Bearer test-key" {
			t.Errorf("unexpected authorization header %q", authorization)
		}
		var payload struct {
			Input      []string `json:"input"`
			Model      string   `json:"model"`
			Dimensions int      `json:"dimensions"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode embedding request: %v", err)
		}
		if payload.Model != "embedding-model" || payload.Dimensions != 2 {
			t.Errorf("unexpected embedding payload: %#v", payload)
		}
		if len(payload.Input) != 2 || payload.Input[0] != "first" || payload.Input[1] != "second" {
			t.Errorf("unexpected embedding inputs: %#v", payload.Input)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[1,2]},{"object":"embedding","index":1,"embedding":[3,4]}],"model":"embedding-model","usage":{"prompt_tokens":2,"total_tokens":2}}`))
	}))
	defer server.Close()

	provider, err := New(Config{
		APIKey: "test-key", BaseURL: server.URL + "/v1", Model: "embedding-model", Dimensions: 2,
		BatchSize: 10, Timeout: 5 * time.Second, RequestDimensions: true, PreserveNewLines: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.EmbedDocuments(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs: []domainknowledge.EmbedInput{
			{ID: "chunk-1", Text: "first"},
			{ID: "chunk-2", Text: "second"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 2 || result.Outputs[1].Vector[1] != 4 {
		t.Fatalf("unexpected HTTP embedding result: %#v", result)
	}
}

func TestProviderPreservesInputIDsAndOrder(t *testing.T) {
	embedder := &providerTestEmbedder{documentVectors: [][]float32{{1, 2}, {3, 4}}}
	provider, err := newWithEmbedder(embedder, 2)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.EmbedDocuments(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs: []domainknowledge.EmbedInput{
			{ID: "chunk-b", Text: "second"},
			{ID: "chunk-a", Text: "first"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outputs) != 2 || result.Outputs[0].ID != "chunk-b" || result.Outputs[1].ID != "chunk-a" {
		t.Fatalf("provider changed input identity or order: %#v", result.Outputs)
	}
	if len(embedder.documentTexts) != 2 || embedder.documentTexts[0] != "second" || embedder.documentTexts[1] != "first" {
		t.Fatalf("unexpected embedded texts: %#v", embedder.documentTexts)
	}
}

func TestProviderRejectsInvalidQueryAndDimensions(t *testing.T) {
	embedder := &providerTestEmbedder{queryVector: []float32{1}}
	provider, err := newWithEmbedder(embedder, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.EmbedQuery(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs: []domainknowledge.EmbedInput{
			{ID: "query-1", Text: "one"},
			{ID: "query-2", Text: "two"},
		},
	})
	if err == nil {
		t.Fatal("expected multiple query inputs to fail")
	}
	_, err = provider.EmbedQuery(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs:             []domainknowledge.EmbedInput{{ID: "query-1", Text: "one"}},
	})
	if err == nil {
		t.Fatal("expected wrong vector dimensions to fail")
	}
}

func TestProviderRejectsDuplicateIDsAndNonFiniteVectors(t *testing.T) {
	embedder := &providerTestEmbedder{documentVectors: [][]float32{{1, 2}, {3, 4}}}
	provider, err := newWithEmbedder(embedder, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.EmbedDocuments(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs: []domainknowledge.EmbedInput{
			{ID: "same", Text: "one"},
			{ID: "same", Text: "two"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate input ids to fail")
	}
	embedder.documentVectors = [][]float32{{float32(math.NaN()), 1}}
	_, err = provider.EmbedDocuments(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs:             []domainknowledge.EmbedInput{{ID: "chunk-1", Text: "one"}},
	})
	if err == nil {
		t.Fatal("expected non-finite vector to fail")
	}
}

type providerTestEmbedder struct {
	documentTexts   []string
	documentVectors [][]float32
	queryText       string
	queryVector     []float32
}

func (embedder *providerTestEmbedder) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	embedder.documentTexts = append([]string(nil), texts...)
	return embedder.documentVectors, nil
}

func (embedder *providerTestEmbedder) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	embedder.queryText = text
	return embedder.queryVector, nil
}
