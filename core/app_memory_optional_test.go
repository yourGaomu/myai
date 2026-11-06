package core

import (
	"testing"

	memoryextractionservice "myai/core/application/memory/extraction/service"
)

func TestUnavailableMemoryExtractionUsesNilInterfaces(t *testing.T) {
	app := &Application{}
	if app.GetMemoryExtractionService() != nil {
		t.Fatal("unavailable extraction service must be a nil interface")
	}
	if app.memoryRunObserver() != nil {
		t.Fatal("unavailable extraction service must not be installed as a completion observer")
	}
}

func TestAvailableMemoryExtractionKeepsService(t *testing.T) {
	extraction := &memoryextractionservice.Service{}
	app := &Application{memoryExtractionService: extraction}
	if app.GetMemoryExtractionService() != extraction {
		t.Fatal("available extraction service was not retained")
	}
	if app.memoryRunObserver() != extraction {
		t.Fatal("available completion observer was not retained")
	}
}
