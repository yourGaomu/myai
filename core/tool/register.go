package tool

import (
	"errors"
	"sort"

	"github.com/tmc/langchaingo/llms"

	tooldef "myai/core/tool/tool"
)

type RegisterTools struct {
	tools map[string]tooldef.Tool
}

func NewRegisterTools() *RegisterTools {
	return &RegisterTools{
		tools: make(map[string]tooldef.Tool),
	}
}

func (rt *RegisterTools) verifyTools() {
	if rt.tools == nil || len(rt.tools) == 0 {
		rt.tools = make(map[string]tooldef.Tool)
	}
}

func (r *RegisterTools) Register(t tooldef.Tool) {
	r.verifyTools()
	if t == nil {
		return
	}
	r.tools[t.Name()] = t
}

func (rt *RegisterTools) GetTool(name string) (tooldef.Tool, error) {
	rt.verifyTools()
	t := rt.tools[name]
	if t == nil {
		return nil, errors.New("Tool " + name + " is not registered")
	}
	return t, nil
}

func (rt *RegisterTools) List() []tooldef.Tool {
	rt.verifyTools()

	names := make([]string, 0, len(rt.tools))
	for name := range rt.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := make([]tooldef.Tool, 0, len(names))
	for _, name := range names {
		tools = append(tools, rt.tools[name])
	}

	return tools
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
