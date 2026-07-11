package langchaingo

import (
	"testing"

	modelport "myai/core/port/model"
)

func TestToolMapperConvertsFunctionDefinition(t *testing.T) {
	tool := modelport.Tool{
		Type: "function",
		Function: &modelport.FunctionDefinition{
			Name:        "read_file",
			Description: "Read a workspace file",
			Parameters: map[string]any{
				"type": "object",
			},
		},
	}

	mapped := ToLLMTool(tool)
	if mapped.Type != "function" {
		t.Fatalf("unexpected tool type: %q", mapped.Type)
	}
	if mapped.Function == nil {
		t.Fatal("expected function definition")
	}
	if mapped.Function.Name != "read_file" || mapped.Function.Description != "Read a workspace file" {
		t.Fatalf("unexpected function definition: %#v", mapped.Function)
	}
	if mapped.Function.Parameters == nil {
		t.Fatal("expected parameters")
	}
}
