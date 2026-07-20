package service

import (
	"context"
	"math"
	"testing"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

func TestResolverValidatesProfileAndNormalizesWithoutMutatingDelegate(t *testing.T) {
	delegate := &resolverTestProvider{result: domainknowledge.EmbedResult{
		EmbeddingProfileID: "profile-1",
		Dimensions:         2,
		Outputs:            []domainknowledge.EmbedOutput{{ID: "chunk-1", Vector: []float32{3, 4}}},
	}}
	resolver := Resolver{Registry: resolverTestRegistry{provider: delegate, info: resolverTestInfo()}}
	provider, err := resolver.Resolve(resolverTestProfile(true))
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.EmbedDocuments(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs:             []domainknowledge.EmbedInput{{ID: "chunk-1", Text: "text"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(result.Outputs[0].Vector[0])-0.6) > 0.00001 || math.Abs(float64(result.Outputs[0].Vector[1])-0.8) > 0.00001 {
		t.Fatalf("unexpected normalized vector: %#v", result.Outputs[0].Vector)
	}
	if delegate.result.Outputs[0].Vector[0] != 3 || delegate.result.Outputs[0].Vector[1] != 4 {
		t.Fatal("resolver must not mutate delegate-owned vectors")
	}
}

func TestResolverRejectsDifferentModelEvenWhenDimensionsMatch(t *testing.T) {
	info := resolverTestInfo()
	info.Model = "different-model"
	resolver := Resolver{Registry: resolverTestRegistry{provider: &resolverTestProvider{}, info: info}}
	if _, err := resolver.Resolve(resolverTestProfile(false)); err == nil {
		t.Fatal("same dimensions must not make different models compatible")
	}
}

func TestResolvedProviderRejectsWrongProfileAndZeroNormalization(t *testing.T) {
	delegate := &resolverTestProvider{result: domainknowledge.EmbedResult{
		EmbeddingProfileID: "profile-1",
		Dimensions:         2,
		Outputs:            []domainknowledge.EmbedOutput{{ID: "chunk-1", Vector: []float32{0, 0}}},
	}}
	resolver := Resolver{Registry: resolverTestRegistry{provider: delegate, info: resolverTestInfo()}}
	provider, err := resolver.Resolve(resolverTestProfile(true))
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.EmbedDocuments(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "wrong-profile",
		Inputs:             []domainknowledge.EmbedInput{{ID: "chunk-1", Text: "text"}},
	})
	if err == nil {
		t.Fatal("expected request profile mismatch")
	}
	_, err = provider.EmbedDocuments(context.Background(), domainknowledge.EmbedRequest{
		EmbeddingProfileID: "profile-1",
		Inputs:             []domainknowledge.EmbedInput{{ID: "chunk-1", Text: "text"}},
	})
	if err == nil {
		t.Fatal("expected zero vector normalization error")
	}
}

type resolverTestRegistry struct {
	provider knowledgeport.EmbeddingProvider
	info     domainknowledge.EmbeddingModelInfo
}

func (registry resolverTestRegistry) Get(string) (knowledgeport.EmbeddingProvider, bool) {
	return registry.provider, registry.provider != nil
}

func (registry resolverTestRegistry) GetInfo(string) (domainknowledge.EmbeddingModelInfo, bool) {
	return registry.info, registry.info.ID != ""
}

func (registry resolverTestRegistry) List() []domainknowledge.EmbeddingModelInfo {
	return []domainknowledge.EmbeddingModelInfo{registry.info}
}

type resolverTestProvider struct {
	result domainknowledge.EmbedResult
}

func (provider *resolverTestProvider) EmbedDocuments(context.Context, domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	return provider.result, nil
}

func (provider *resolverTestProvider) EmbedQuery(context.Context, domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	return provider.result, nil
}

func resolverTestInfo() domainknowledge.EmbeddingModelInfo {
	return domainknowledge.EmbeddingModelInfo{
		ID: "model-1", Name: "Model", Provider: "openai-compatible", Model: "embedding-model", ModelVersion: "v1", Dimensions: 2, Enabled: true,
	}
}

func resolverTestProfile(normalize bool) domainknowledge.EmbeddingProfile {
	return domainknowledge.EmbeddingProfile{
		ID: "profile-1", Name: "Profile", ModelID: "model-1", Provider: "openai-compatible", Model: "embedding-model", ModelVersion: "v1", Dimensions: 2, Normalize: normalize,
	}
}
