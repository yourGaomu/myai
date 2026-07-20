package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"

	domainknowledge "myai/core/domain/knowledge"
)

func TestRegistryStoresMetadataAndListsByID(t *testing.T) {
	registry := NewRegistry()
	provider := registryTestProvider{}
	for _, id := range []string{"model-b", "model-a"} {
		if err := registry.Set(id, provider, registryTestInfo(id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, exists := registry.Get("model-a"); !exists {
		t.Fatal("expected registered provider")
	}
	info, exists := registry.GetInfo("model-b")
	if !exists || info.ID != "model-b" || info.Dimensions != 3 {
		t.Fatalf("unexpected model info: %#v", info)
	}
	listed := registry.List()
	if len(listed) != 2 || listed[0].ID != "model-a" || listed[1].ID != "model-b" {
		t.Fatalf("expected sorted model list: %#v", listed)
	}
}

func TestRegistryRejectsMismatchedMetadata(t *testing.T) {
	registry := NewRegistry()
	info := registryTestInfo("different")
	if err := registry.Set("model-a", registryTestProvider{}, info); err == nil {
		t.Fatal("expected mismatched model id error")
	}
	if err := registry.Set("model-a", nil, registryTestInfo("model-a")); err == nil {
		t.Fatal("expected nil provider error")
	}
}

func TestRegistrySupportsConcurrentRegistrationAndReads(t *testing.T) {
	registry := NewRegistry()
	var wait sync.WaitGroup
	for index := range 32 {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			id := fmt.Sprintf("model-%02d", index)
			if err := registry.Set(id, registryTestProvider{}, registryTestInfo(id)); err != nil {
				t.Errorf("register %s: %v", id, err)
				return
			}
			if _, exists := registry.Get(id); !exists {
				t.Errorf("provider %s disappeared", id)
			}
		}()
	}
	wait.Wait()
	if len(registry.List()) != 32 {
		t.Fatalf("unexpected registered model count: %d", len(registry.List()))
	}
}

func registryTestInfo(id string) domainknowledge.EmbeddingModelInfo {
	return domainknowledge.EmbeddingModelInfo{
		ID: id, Name: id, Provider: "test", Model: id, Dimensions: 3, Enabled: true,
	}
}

type registryTestProvider struct{}

func (registryTestProvider) EmbedDocuments(context.Context, domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	return domainknowledge.EmbedResult{}, nil
}

func (registryTestProvider) EmbedQuery(context.Context, domainknowledge.EmbedRequest) (domainknowledge.EmbedResult, error) {
	return domainknowledge.EmbedResult{}, nil
}
