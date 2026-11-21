package langchaingo

import (
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/ai/generativelanguage/apiv1beta/generativelanguagepb"
	"github.com/tmc/langchaingo/llms"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// geminiDefaultCallOptions matches the pinned LangChainGo googleai defaults.
// Apply call options after these defaults so explicitly set zeros are preserved.
func geminiDefaultCallOptions(modelName string) llms.CallOptions {
	return llms.CallOptions{
		Model:          modelName,
		CandidateCount: 1,
		MaxTokens:      2048,
		Temperature:    0.5,
		TopK:           3,
		TopP:           0.95,
	}
}

// geminiRequest builds the protobuf request from resolved call options.
func geminiRequest(modelName string, messages []llms.MessageContent, opts llms.CallOptions) (*generativelanguagepb.GenerateContentRequest, error) {
	if opts.Model != "" {
		modelName = opts.Model
	}
	if !strings.ContainsRune(modelName, '/') {
		modelName = "models/" + modelName
	}

	tools, err := geminiRequestTools(opts.Tools)
	if err != nil {
		return nil, err
	}
	if opts.JSONMode {
		if opts.ResponseMIMEType != "" {
			return nil, fmt.Errorf("conflicting options, can't use JSONMode and ResponseMIMEType together")
		}
		opts.ResponseMIMEType = "application/json"
	}

	candidateCount := int32(opts.CandidateCount)
	if len(messages) == 1 {
		if messages[0].Role != llms.ChatMessageTypeHuman {
			return nil, fmt.Errorf("got %v message role, want human", messages[0].Role)
		}
	} else {
		// Both SendMessage and SendMessageStream forced a single candidate.
		candidateCount = 1
	}
	req := &generativelanguagepb.GenerateContentRequest{
		Model:    modelName,
		Contents: make([]*generativelanguagepb.Content, 0, len(messages)),
		Tools:    tools,
		GenerationConfig: &generativelanguagepb.GenerationConfig{
			CandidateCount:   proto.Int32(candidateCount),
			MaxOutputTokens:  proto.Int32(int32(opts.MaxTokens)),
			Temperature:      proto.Float32(float32(opts.Temperature)),
			TopP:             proto.Float32(float32(opts.TopP)),
			TopK:             proto.Int32(int32(opts.TopK)),
			StopSequences:    opts.StopWords,
			ResponseMimeType: opts.ResponseMIMEType,
		},
	}
	for _, category := range []generativelanguagepb.HarmCategory{
		generativelanguagepb.HarmCategory_HARM_CATEGORY_DANGEROUS_CONTENT,
		generativelanguagepb.HarmCategory_HARM_CATEGORY_HARASSMENT,
		generativelanguagepb.HarmCategory_HARM_CATEGORY_HATE_SPEECH,
		generativelanguagepb.HarmCategory_HARM_CATEGORY_SEXUALLY_EXPLICIT,
	} {
		req.SafetySettings = append(req.SafetySettings, &generativelanguagepb.SafetySetting{
			Category:  category,
			Threshold: generativelanguagepb.SafetySetting_BLOCK_ONLY_HIGH,
		})
	}
	if name, ok := opts.Metadata["CachedContentName"].(string); ok && name != "" {
		req.CachedContent = proto.String(name)
	}

	for i, message := range messages {
		content := &generativelanguagepb.Content{}
		switch message.Role {
		case llms.ChatMessageTypeSystem:
			content.Role = "system"
		case llms.ChatMessageTypeAI:
			content.Role = "model"
		case llms.ChatMessageTypeHuman, llms.ChatMessageTypeGeneric, llms.ChatMessageTypeTool:
			content.Role = "user"
		default:
			return nil, fmt.Errorf("message %d: role %v not supported", i, message.Role)
		}
		for j, part := range message.Parts {
			converted, err := geminiRequestPart(part)
			if err != nil {
				return nil, fmt.Errorf("message %d, part %d: %w", i, j, err)
			}
			content.Parts = append(content.Parts, converted)
		}
		if message.Role == llms.ChatMessageTypeSystem {
			req.SystemInstruction = content // The last system message wins.
		} else {
			req.Contents = append(req.Contents, content)
		}
	}
	if len(req.Contents) == 0 {
		return nil, fmt.Errorf("Gemini request requires at least one non-system message")
	}
	// The old chat session used only the final message's parts and assigned it
	// the user role, regardless of its original role.
	req.Contents[len(req.Contents)-1].Role = "user"
	return req, nil
}

func geminiRequestPart(part llms.ContentPart) (*generativelanguagepb.Part, error) {
	switch p := part.(type) {
	case llms.TextContent:
		return &generativelanguagepb.Part{Data: &generativelanguagepb.Part_Text{Text: p.Text}}, nil
	case llms.BinaryContent:
		return &generativelanguagepb.Part{Data: &generativelanguagepb.Part_InlineData{
			InlineData: &generativelanguagepb.Blob{MimeType: p.MIMEType, Data: p.Data},
		}}, nil
	case llms.ToolCall:
		if p.FunctionCall == nil {
			return nil, fmt.Errorf("tool call is missing its function")
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(p.FunctionCall.Arguments), &values); err != nil {
			return nil, fmt.Errorf("decode function arguments: %w", err)
		}
		var args *structpb.Struct
		if values != nil {
			var err error
			args, err = structpb.NewStruct(values)
			if err != nil {
				return nil, fmt.Errorf("encode function arguments: %w", err)
			}
		}
		return &generativelanguagepb.Part{Data: &generativelanguagepb.Part_FunctionCall{
			FunctionCall: &generativelanguagepb.FunctionCall{Name: p.FunctionCall.Name, Args: args},
		}}, nil
	case llms.ToolCallResponse:
		// Preserve the string result even if it contains JSON; the legacy
		// adapter did not decode it into a structured value.
		response, err := structpb.NewStruct(map[string]any{"response": p.Content})
		if err != nil {
			return nil, fmt.Errorf("encode function response: %w", err)
		}
		return &generativelanguagepb.Part{Data: &generativelanguagepb.Part_FunctionResponse{
			FunctionResponse: &generativelanguagepb.FunctionResponse{Name: p.Name, Response: response},
		}}, nil
	case llms.ImageURLContent:
		return nil, fmt.Errorf("Gemini request does not fetch image URLs; use binary content instead")
	default:
		return nil, fmt.Errorf("unsupported Gemini content part %T", part)
	}
}

func geminiRequestTools(tools []llms.Tool) ([]*generativelanguagepb.Tool, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	declarations := make([]*generativelanguagepb.FunctionDeclaration, 0, len(tools))
	for i, tool := range tools {
		if tool.Type != "function" {
			return nil, fmt.Errorf("tool [%d]: unsupported type %q, want 'function'", i, tool.Type)
		}
		if tool.Function == nil {
			return nil, fmt.Errorf("tool [%d]: missing function definition", i)
		}
		params, ok := tool.Function.Parameters.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tool [%d]: unsupported type %T of Parameters", i, tool.Function.Parameters)
		}
		schema, err := geminiRequestSchema(params, "")
		if err != nil {
			return nil, fmt.Errorf("tool [%d]: %w", i, err)
		}
		declarations = append(declarations, &generativelanguagepb.FunctionDeclaration{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  schema,
		})
	}
	return []*generativelanguagepb.Tool{{FunctionDeclarations: declarations}}, nil
}

// geminiRequestSchema converts the same subset of JSON Schema as LangChainGo:
// type, description, properties, array items, and required property names.
func geminiRequestSchema(fields map[string]any, path string) (*generativelanguagepb.Schema, error) {
	schema := &generativelanguagepb.Schema{}
	if value, exists := fields["type"]; exists {
		name, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("property [%s]: expected string for type", path)
		}
		switch name {
		case "object":
			schema.Type = generativelanguagepb.Type_OBJECT
		case "array":
			schema.Type = generativelanguagepb.Type_ARRAY
		case "string":
			schema.Type = generativelanguagepb.Type_STRING
		case "number":
			schema.Type = generativelanguagepb.Type_NUMBER
		case "integer":
			schema.Type = generativelanguagepb.Type_INTEGER
		case "boolean":
			schema.Type = generativelanguagepb.Type_BOOLEAN
		}
	}
	if value, exists := fields["description"]; exists {
		var ok bool
		schema.Description, ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("property [%s]: expected string for description", path)
		}
	}
	if value, exists := fields["properties"]; exists {
		properties, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("property [%s]: expected map for properties", path)
		}
		schema.Properties = make(map[string]*generativelanguagepb.Schema, len(properties))
		for name, value := range properties {
			childPath := name
			if path != "" {
				childPath = path + "." + name
			}
			child, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("property [%s]: expected map for property schema", childPath)
			}
			converted, err := geminiRequestSchema(child, childPath)
			if err != nil {
				return nil, err
			}
			schema.Properties[name] = converted
		}
	} else if schema.Type == generativelanguagepb.Type_OBJECT && path == "" {
		return nil, fmt.Errorf("expected to find a map of properties")
	}
	if value, exists := fields["items"]; exists && schema.Type == generativelanguagepb.Type_ARRAY {
		items, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("property [%s]: expected map for array items", path)
		}
		var err error
		schema.Items, err = geminiRequestSchema(items, path+"[]")
		if err != nil {
			return nil, err
		}
	}
	if value, exists := fields["required"]; exists {
		switch required := value.(type) {
		case []string:
			schema.Required = required
		case []any:
			schema.Required = make([]string, 0, len(required))
			for _, value := range required {
				name, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("property [%s]: expected string for required", path)
				}
				schema.Required = append(schema.Required, name)
			}
		default:
			return nil, fmt.Errorf("property [%s]: expected array for required", path)
		}
	}
	return schema, nil
}
