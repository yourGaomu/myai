package langchaingo

import (
	"fmt"
	"sync"

	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

// Factory selects a protocol adapter and delegates model construction to it.
// It is the composition boundary between the application layer and
// LangChainGo-specific protocol implementations.
type Factory struct {
	mu       sync.RWMutex
	adapters map[domainmodel.Protocol]modelport.ProtocolAdapter
}

func NewFactory(adapters ...modelport.ProtocolAdapter) *Factory {
	factory := &Factory{adapters: make(map[domainmodel.Protocol]modelport.ProtocolAdapter)}
	adapters = append([]modelport.ProtocolAdapter{
		OpenAIChatCompletionsAdapter{},
		AnthropicMessagesAdapter{},
		GoogleGenerativeAIAdapter{},
		MistralChatAdapter{},
		OllamaChatAdapter{},
	}, adapters...)
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		factory.adapters[domainmodel.NormalizeProtocol(adapter.Protocol())] = adapter
	}
	return factory
}

func (f *Factory) CreateModel(config modelport.CreationConfig) (modelport.ChatModelPort, error) {
	protocol := domainmodel.NormalizeProtocol(config.Protocol)
	adapter, ok := f.adapter(protocol)
	if !ok {
		return nil, fmt.Errorf("unsupported model protocol: %s", protocol)
	}
	return adapter.CreateModel(config)
}

func (f *Factory) ValidateConfig(config modelport.CreationConfig) error {
	protocol := domainmodel.NormalizeProtocol(config.Protocol)
	adapter, ok := f.adapter(protocol)
	if !ok {
		return fmt.Errorf("unsupported model protocol: %s", protocol)
	}
	return adapter.ValidateConfig(config)
}

func (f *Factory) SupportsProtocol(protocol domainmodel.Protocol) bool {
	if f == nil {
		return false
	}
	_, ok := f.adapter(domainmodel.NormalizeProtocol(protocol))
	return ok
}

func (f *Factory) Register(adapter modelport.ProtocolAdapter) error {
	if f == nil {
		return fmt.Errorf("model factory is nil")
	}
	if adapter == nil {
		return fmt.Errorf("protocol adapter is nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.adapters == nil {
		f.adapters = make(map[domainmodel.Protocol]modelport.ProtocolAdapter)
	}
	f.adapters[domainmodel.NormalizeProtocol(adapter.Protocol())] = adapter
	return nil
}

func (f *Factory) adapter(protocol domainmodel.Protocol) (modelport.ProtocolAdapter, bool) {
	if f == nil {
		return nil, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.adapters == nil {
		if protocol == domainmodel.ProtocolOpenAIChatCompletions {
			return OpenAIChatCompletionsAdapter{}, true
		}
		return nil, false
	}
	adapter, ok := f.adapters[protocol]
	return adapter, ok
}
