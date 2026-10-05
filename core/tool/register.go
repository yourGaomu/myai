package tool

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	modelport "myai/core/port/model"
	tooldef "myai/core/tool/tool"
)

type RegisterTools struct {
	mu        sync.RWMutex
	turns     sync.Mutex
	leases    map[*RegisterTools][]func()
	sources   map[string]map[string]tooldef.Tool
	flatTools []tooldef.Tool
	flatMap   map[string]tooldef.Tool
}

// Snapshot pins a private tool catalog. Publishing a new catalog never waits
// for active turns; retired processes close after their last snapshot releases.
func (rt *RegisterTools) Snapshot() (*RegisterTools, func()) {
	if rt == nil {
		return NewRegisterTools(), func() {}
	}
	rt.turns.Lock()
	snapshot := rt.CloneExcludingSources(nil)
	if rt.leases == nil {
		rt.leases = make(map[*RegisterTools][]func())
	}
	rt.leases[snapshot] = nil
	rt.turns.Unlock()
	var once sync.Once
	return snapshot, func() {
		once.Do(func() {
			rt.turns.Lock()
			callbacks := rt.leases[snapshot]
			delete(rt.leases, snapshot)
			rt.turns.Unlock()
			for _, callback := range callbacks {
				callback()
			}
		})
	}
}

// Retire must be called while BeginReload is held, after publishing/removing
// sources. Only snapshots acquired before this publication delay cleanup.
func (rt *RegisterTools) Retire(cleanup func()) {
	var remaining atomic.Int64
	remaining.Store(int64(len(rt.leases)))
	if remaining.Load() == 0 {
		cleanup()
		return
	}
	for snapshot := range rt.leases {
		rt.leases[snapshot] = append(rt.leases[snapshot], func() {
			if remaining.Add(-1) == 0 {
				cleanup()
			}
		})
	}
}

func (rt *RegisterTools) BeginReload() func() {
	if rt == nil {
		return func() {}
	}
	rt.turns.Lock()
	return rt.turns.Unlock
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

// UnregisterSources removes several sources and rebuilds the flattened view
// once, so readers cannot observe a half-removed runtime set.
func (rt *RegisterTools) UnregisterSources(sources []string) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.ensureLocked()
	for _, source := range sources {
		if source == "" {
			source = "local"
		}
		delete(rt.sources, source)
	}
	rt.rebuildLocked()
}

// SourceToolNames returns the names currently owned by a source. It is used
// by runtime reloaders to distinguish their replacement tools from unrelated
// local or plugin tools when resolving name collisions.
func (rt *RegisterTools) SourceToolNames(source string) []string {
	if rt == nil {
		return nil
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	items := rt.sources[source]
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SourceTools returns a copy of the tools owned by one source.
func (rt *RegisterTools) SourceTools(source string) []tooldef.Tool {
	if rt == nil {
		return nil
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	items := rt.sources[source]
	tools := make([]tooldef.Tool, 0, len(items))
	for _, item := range items {
		if item != nil {
			tools = append(tools, item)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
	return tools
}

// CloneExcludingSources creates a private registry snapshot. It is intended
// for validating a complete replacement before publishing it to the live
// registry.
func (rt *RegisterTools) CloneExcludingSources(excluded map[string]bool) *RegisterTools {
	clone := NewRegisterTools()
	if rt == nil {
		return clone
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for source, items := range rt.sources {
		if excluded != nil && excluded[source] {
			continue
		}
		tools := make([]tooldef.Tool, 0, len(items))
		for _, item := range items {
			tools = append(tools, item)
		}
		clone.sources[source] = make(map[string]tooldef.Tool, len(tools))
		for _, item := range tools {
			if item != nil {
				clone.sources[source][item.Name()] = item
			}
		}
	}
	clone.rebuildLocked()
	return clone
}

// ReplaceSources publishes selected sources from a validated snapshot while
// holding one registry lock, so model tool discovery cannot observe a partial
// plugin replacement.
func (rt *RegisterTools) ReplaceSources(snapshot *RegisterTools, remove []string, add []string) {
	if rt == nil || snapshot == nil {
		return
	}
	copySources := make(map[string]map[string]tooldef.Tool, len(add))
	snapshot.mu.RLock()
	for _, source := range add {
		items := snapshot.sources[source]
		copied := make(map[string]tooldef.Tool, len(items))
		for name, item := range items {
			copied[name] = item
		}
		copySources[source] = copied
	}
	snapshot.mu.RUnlock()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.ensureLocked()
	for _, source := range remove {
		delete(rt.sources, source)
	}
	for source, items := range copySources {
		rt.sources[source] = items
	}
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

func (rt *RegisterTools) LLMTools() []modelport.Tool {
	return rt.LLMToolsByPermission(nil)
}

func (rt *RegisterTools) LLMToolsByPermission(allow func(tooldef.Permission) bool) []modelport.Tool {
	registered := rt.List()
	tools := make([]modelport.Tool, 0, len(registered))

	for _, t := range registered {
		permission := tooldef.NormalizePermission(t.Permission())
		if allow != nil && !allow(permission) {
			continue
		}
		tools = append(tools, llmToolFromRegistered(t))
	}

	return tools
}

func llmToolFromRegistered(t tooldef.Tool) modelport.Tool {
	return modelport.Tool{
		Type: "function",
		Function: &modelport.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		},
	}
}
