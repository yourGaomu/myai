package model

type Tool struct {
	Type     string
	Function *FunctionDefinition
}

type FunctionDefinition struct {
	Name        string
	Description string
	Parameters  any
}
