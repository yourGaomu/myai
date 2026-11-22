package model

type Tool struct {
	Type     string
	Function *FunctionDefinition
	origin   ToolOrigin
}

// ToolOrigin records provenance supplied by a registered tool implementation.
type ToolOrigin uint8

const (
	ToolOriginMCP ToolOrigin = iota + 1
	ToolOriginBuiltinAssetReader
)

func (t Tool) WithOrigin(origin ToolOrigin) Tool {
	t.origin = origin
	return t
}

func (t Tool) IsMCP() bool {
	return t.origin == ToolOriginMCP
}

func (t Tool) IsBuiltinAssetReader() bool {
	return t.origin == ToolOriginBuiltinAssetReader
}

type FunctionDefinition struct {
	Name        string
	Description string
	Parameters  any
}
