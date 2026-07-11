package service

import (
	"testing"

	modelport "myai/core/port/model"
)

func TestQueryServiceListsModelCopies(t *testing.T) {
	catalog := &queryModelCatalog{models: []modelport.ModelInfo{{ID: "model-a"}}}

	result := (QueryService{Catalog: catalog}).ListModels()
	if len(result.Models) != 1 || result.Models[0].ID != "model-a" {
		t.Fatalf("unexpected models: %#v", result.Models)
	}
	result.Models[0].ID = "changed"
	if catalog.models[0].ID != "model-a" {
		t.Fatal("expected query result to own its model slice")
	}
}

func TestQueryServiceReturnsEmptyResultWithoutCatalog(t *testing.T) {
	result := (QueryService{}).ListModels()
	if len(result.Models) != 0 {
		t.Fatalf("expected empty result, got %#v", result.Models)
	}
}

type queryModelCatalog struct {
	models []modelport.ModelInfo
}

func (c *queryModelCatalog) ListModels() []modelport.ModelInfo {
	return c.models
}
