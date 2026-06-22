package tool

import (
	"fmt"
	"sort"
	"sync"

	"github.com/tmc/langchaingo/llms"

	tooldef "myai/core/tool/tool"
)

type RegisterTools struct {
	mu        sync.RWMutex
	sources   map[string]map[string]tooldef.Tool
	flatTools []tooldef.Tool
	flatMap   map[string]tooldef.Tool
}

func NewRegisterTools() *RegisterTools {
	return &RegisterTools{
		sources: map[string]map[string]tooldef.Tool{
			"local": {},
		},
		flatMap: make(map[string]tooldef.Tool),
	}
}

func (rt *RegisterTools) ensureLocked() {
	if rt.sources == nil {
		rt.sources = make(map[string]map[string]tooldef.Tool)
	}
	if rt.sources["local"] == nil {
		rt.sources["local"] = make(map[string]tooldef.Tool)
	}
	if rt.flatMap == nil {
		rt.flatMap = make(map[string]tooldef.Tool)
	}
}

func (rt *RegisterTools) Register(t tooldef.Tool) {
	if t == nil {
		return
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.ensureLocked()
	rt.sources["local"][t.Name()] = t
	rt.rebuildLocked()
}

func (rt *RegisterTools) RegisterSource(source string, tools []tooldef.Tool) {
	if source == "" {
		source = "local"
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.ensureLocked()
	next := make(map[string]tooldef.Tool)
	for _, t := range tools {
		if t == nil {
			continue
		}
		next[t.Name()] = t
	}
	rt.sources[source] = next
	rt.rebuildLocked()
}

func (rt *RegisterTools) UnregisterSource(source string) {
	if source == "" {
		source = "local"
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.ensureLocked()
	delete(rt.sources, source)
	rt.rebuildLocked()
}

func (rt *RegisterTools) GetTool(name string) (tooldef.Tool, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if rt.flatMap != nil {
		if t := rt.flatMap[name]; t != nil {
			return t, nil
		}
	}

	if localTools := rt.sources["local"]; localTools != nil {
		if t := localTools[name]; t != nil {
			return t, nil
		}
	}

	sourceNames := sortedSourceNames(rt.sources, map[string]bool{"local": true})
	for _, source := range sourceNames {
		if t := rt.sources[source][name]; t != nil {
			return t, nil
		}
	}

	return nil, fmt.Errorf("tool %s is not registered", name)
}

func (rt *RegisterTools) List() []tooldef.Tool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	tools := make([]tooldef.Tool, len(rt.flatTools))
	copy(tools, rt.flatTools)
	return tools
}

func (rt *RegisterTools) rebuildLocked() {
	rt.flatMap = make(map[string]tooldef.Tool)
	rt.flatTools = rt.flatTools[:0]

	rt.addSourceLocked("local")

	sourceNames := sortedSourceNames(rt.sources, map[string]bool{"local": true})
	for _, source := range sourceNames {
		rt.addSourceLocked(source)
	}
}

func (rt *RegisterTools) addSourceLocked(source string) {
	tools := rt.sources[source]
	if len(tools) == 0 {
		return
	}

	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t := tools[name]
		if t == nil {
			continue
		}
		if rt.flatMap[name] != nil {
			continue
		}
		rt.flatMap[name] = t
		rt.flatTools = append(rt.flatTools, t)
	}
}

func sortedSourceNames(sources map[string]map[string]tooldef.Tool, excluded map[string]bool) []string {
	sourceNames := make([]string, 0, len(sources))
	for source := range sources {
		if excluded[source] {
			continue
		}
		sourceNames = append(sourceNames, source)
	}
	sort.Strings(sourceNames)
	return sourceNames
}

func (rt *RegisterTools) LLMTools() []llms.Tool {
	return rt.LLMToolsByPermission(nil)
}

func (rt *RegisterTools) LLMToolsByPermission(allow func(tooldef.Permission) bool) []llms.Tool {
	registered := rt.List()
	tools := make([]llms.Tool, 0, len(registered))

	for _, t := range registered {
		permission := tooldef.NormalizePermission(t.Permission())
		if allow != nil && !allow(permission) {
			continue
		}
		tools = append(tools, llmToolFromRegistered(t))
	}

	return tools
}

func llmToolFromRegistered(t tooldef.Tool) llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		},
	}
}
