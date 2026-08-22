package langchaingo

import (
	"context"
	"testing"

	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

func TestFactoryResolvesAdapterByProtocol(t *testing.T) {
	adapter := &recordingAdapter{protocol: domainmodel.Protocol("custom")}
	factory := NewFactory(adapter)

	if !factory.SupportsProtocol(" custom ") {
		t.Fatal("expected custom protocol to be supported")
	}
	if _, err := factory.CreateModel(modelport.CreationConfig{Protocol: "custom"}); err != nil {
		t.Fatal(err)
	}
	if !adapter.called {
		t.Fatal("expected the registered adapter to create the model")
	}
}

func TestFactoryRejectsUnregisteredProtocol(t *testing.T) {
	factory := NewFactory(&recordingAdapter{protocol: "custom"})

	_, err := factory.CreateModel(modelport.CreationConfig{Protocol: "unknown"})
	if err == nil || err.Error() != "unsupported model protocol: unknown" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestZeroValueFactoryKeepsOpenAICompatibility(t *testing.T) {
	var factory Factory
	if !factory.SupportsProtocol(domainmodel.ProtocolOpenAIChatCompletions) {
		t.Fatal("expected zero-value factory to register the built-in adapter")
	}
}

type recordingAdapter struct {
	protocol domainmodel.Protocol
	called   bool
}

func (a *recordingAdapter) Protocol() domainmodel.Protocol {
	return a.protocol
}

func (a *recordingAdapter) CreateModel(modelport.CreationConfig) (modelport.ChatModelPort, error) {
	a.called = true
	return recordingModel{}, nil
}

type recordingModel struct{}

func (recordingModel) Generate(context.Context, modelport.GenerateRequest) (modelport.ChatResult, error) {
	return modelport.ChatResult{}, nil
}
