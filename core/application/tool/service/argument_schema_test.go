package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	toolcommand "myai/core/application/tool/command"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/session"
	tooldef "myai/core/tool/tool"
)

func TestValidateToolArgumentsChecksSchemaConstraints(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"name":    map[string]any{"type": "string", "minLength": 1, "maxLength": 3},
			"count":   map[string]any{"type": "integer", "minimum": 1, "maximum": 3},
			"enabled": map[string]any{"type": "boolean"},
			"mode":    map[string]any{"type": "string", "enum": []string{"read", "write"}},
			"items": map[string]any{"type": "array", "minItems": 1, "maxItems": 2,
				"items": map[string]any{"type": "object", "additionalProperties": false,
					"properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}},
		}, "required": []string{"name", "count"},
	}
	for _, test := range []struct {
		name, arguments, failure string
	}{
		{"valid", `{"name":"好","count":2,"enabled":true,"mode":"read","items":[{"path":"a"}]}`, ""},
		{"minimal", `{"name":"abc","count":1}`, ""},
		{"missing", `{"name":"abc"}`, "arguments.count is required"},
		{"wrong type", `{"name":false,"count":1}`, "arguments.name"},
		{"fraction", `{"name":"a","count":1.5}`, "arguments.count"},
		{"below minimum", `{"name":"a","count":0}`, "minimum"},
		{"above maximum", `{"name":"a","count":4}`, "maximum"},
		{"short string", `{"name":"","count":1}`, "minLength"},
		{"long string", `{"name":"abcd","count":1}`, "maxLength"},
		{"extra property", `{"name":"a","count":1,"extra":true}`, "arguments.extra"},
		{"enum", `{"name":"a","count":1,"mode":"invalid"}`, "enum"},
		{"boolean", `{"name":"a","count":1,"enabled":"true"}`, "arguments.enabled"},
		{"array type", `{"name":"a","count":1,"items":{}}`, "arguments.items"},
		{"empty array", `{"name":"a","count":1,"items":[]}`, "minItems"},
		{"long array", `{"name":"a","count":1,"items":[{},{},{}]}`, "maxItems"},
		{"nested required", `{"name":"a","count":1,"items":[{}]}`, "items[0].path is required"},
		{"nested type", `{"name":"a","count":1,"items":[{"path":3}]}`, "items[0].path"},
		{"nested extra", `{"name":"a","count":1,"items":[{"path":"a","other":3}]}`, "items[0].other"},
		{"non-object", `[]`, "must be a JSON object"},
		{"null", `null`, "must be a JSON object"},
		{"malformed", `{"name":`, "must be a JSON object"},
		{"trailing JSON", `{} {}`, "must be a JSON object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateToolArguments(schema, test.arguments)
			if test.failure == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.failure) {
				t.Fatalf("expected %q, got %v", test.failure, err)
			}
		})
	}
}

func TestValidateToolArgumentsSupportsSchemaRepresentations(t *testing.T) {
	for _, schema := range []any{
		nil,
		true,
		map[string]any{"type": "object"},
		json.RawMessage(`{"type":"object","properties":{"value":{"type":["string","null"]},"scores":{"type":"object","additionalProperties":{"type":"number","exclusiveMinimum":0,"exclusiveMaximum":10}}}}`),
	} {
		if err := validateToolArguments(schema, `{"value":null,"scores":{"score":1.5}}`); err != nil {
			t.Fatalf("schema %T was not accepted: %v", schema, err)
		}
	}
	for _, test := range []struct {
		schema any
		args   string
	}{
		{false, `{}`},
		{map[string]any{"properties": map[string]any{"value": false}}, `{"value":true}`},
		{map[string]any{"properties": map[string]any{"value": map[string]any{"type": []string{"string", "null"}}}}, `{"value":1}`},
		{json.RawMessage(`{"properties":{"values":{"type":"object","additionalProperties":{"type":"number"}}}}`), `{"values":{"score":"1"}}`},
		{json.RawMessage(`{"properties":{"value":{"exclusiveMinimum":0,"exclusiveMaximum":10}}}`), `{"value":0}`},
		{json.RawMessage(`{"properties":{"value":{"exclusiveMinimum":0,"exclusiveMaximum":10}}}`), `{"value":10}`},
		{json.RawMessage(`{"properties":{"value":{"const":"fixed"}}}`), `{"value":"other"}`},
		{json.RawMessage(`{"properties":{"value":{"anyOf":[{"type":"string"},{"type":"null"}]}}}`), `{"value":1}`},
		{json.RawMessage(`{"properties":{"value":{"allOf":[{"type":"integer"},{"minimum":2}]}}}`), `{"value":1}`},
		{json.RawMessage(`{"properties":{"value":{"oneOf":[{"type":"number"},{"type":"integer"}]}}}`), `{"value":1}`},
		{make(chan int), `{}`},
		{json.RawMessage(`"invalid schema"`), `{}`},
	} {
		if err := validateToolArguments(test.schema, test.args); err == nil {
			t.Fatalf("accepted invalid rewrite for schema %v: %s", test.schema, test.args)
		}
	}
}

func TestExecutionValidatesFinalHookRewriteBeforeApproval(t *testing.T) {
	for _, test := range []struct {
		name, rewrite string
		valid         bool
	}{
		{"valid", `{"path":"updated","lines":[1,2]}`, true},
		{"missing required", `{"lines":[1]}`, false},
		{"wrong type", `{"path":2}`, false},
		{"extra property", `{"path":"updated","extra":true}`, false},
		{"invalid item", `{"path":"updated","lines":["one"]}`, false},
		{"non-object", `[]`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls, asks, results := 0, 0, 0
			var receivedArguments string
			tool := executionTestTool{name: "write", permission: tooldef.PermissionWrite,
				schema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"path"},
					"properties": map[string]any{"path": map[string]any{"type": "string"}, "lines": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}}},
				call: func(_ context.Context, args json.RawMessage) (domaintool.ToolOutput, error) {
					calls++
					receivedArguments = string(args)
					return domaintool.SuccessOutput("done"), nil
				}}
			result, err := (ExecutionService{Registry: executionTestRegistry{"write": tool}, Hooks: executionTestHooks{arguments: test.rewrite}}).Execute(context.Background(), toolcommand.Execution{
				SessionID: "session", PermissionMode: session.PermissionModeAsk,
				Calls: []domainmessage.ToolCall{{ID: "call", Name: "write", Arguments: `{"path":"original"}`}},
				Callbacks: toolcommand.ExecutionCallbacks{
					OnToolAsk: func(request toolcommand.PermissionRequest) bool {
						asks++
						if request.Arguments != test.rewrite {
							t.Error("approval saw stale arguments")
						}
						return true
					},
					OnToolResult: func(string, string, domaintool.ToolOutput) { results++ },
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if results != 1 || len(result.Entries) != 2 || result.Calls[0].Arguments != test.rewrite {
				t.Fatalf("rewrite lifecycle changed: %#v, results=%d", result, results)
			}
			if test.valid {
				if calls != 1 || asks != 1 || receivedArguments != test.rewrite || result.Entries[1].Status != domaintool.ResultStatusSuccess {
					t.Fatalf("valid rewrite did not execute: calls=%d asks=%d result=%#v", calls, asks, result)
				}
			} else if calls != 0 || asks != 0 || result.Entries[1].ErrorCode != "tool_arguments_invalid" {
				t.Fatalf("invalid rewrite reached approval or tool: calls=%d asks=%d result=%#v", calls, asks, result)
			}
		})
	}
}
