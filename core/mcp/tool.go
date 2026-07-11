package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	tooldef "myai/core/tool/tool"
)

var unsupportedToolNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

type Tool struct {
	// Tool 把 MCP ToolInfo 适配为项目通用 Tool，并继承服务器配置的权限等级。
	client       *Client
	serverName   string
	exposedName  string
	originalName string
	title        string
	description  string
	inputSchema  json.RawMessage
	permission   tooldef.Permission
}

func NewTool(client *Client, serverName string, info ToolInfo, permission tooldef.Permission) *Tool {
	return NewToolWithName(client, serverName, info, ExposedToolName(serverName, info.Name), permission)
}

func NewToolWithName(client *Client, serverName string, info ToolInfo, exposedName string, permission tooldef.Permission) *Tool {
	return &Tool{
		client:       client,
		serverName:   serverName,
		exposedName:  exposedName,
		originalName: info.Name,
		title:        info.Title,
		description:  info.Description,
		inputSchema:  info.InputSchema,
		permission:   tooldef.NormalizePermission(permission),
	}
}

func (t *Tool) Name() string {
	return t.exposedName
}

func (t *Tool) Description() string {
	description := strings.TrimSpace(t.description)
	if description == "" {
		description = strings.TrimSpace(t.title)
	}
	if description == "" {
		description = "MCP tool " + t.originalName
	}
	return fmt.Sprintf("[MCP:%s original=%s] %s", t.serverName, t.originalName, description)
}

func (t *Tool) Schema() any {
	if len(t.inputSchema) == 0 {
		return emptyObjectSchema()
	}

	var schema any
	if err := json.Unmarshal(t.inputSchema, &schema); err != nil {
		return emptyObjectSchema()
	}
	if schema == nil {
		return emptyObjectSchema()
	}
	return schema
}

func (t *Tool) Permission() tooldef.Permission {
	return t.permission
}

func (t *Tool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	result, err := t.client.CallTool(ctx, t.originalName, args)
	if err != nil {
		return "", err
	}

	output := formatCallResult(result)
	if result.IsError {
		if output == "" {
			output = "mcp tool returned an error"
		}
		return "", fmt.Errorf("%s", output)
	}
	return output, nil
}

func ExposedToolName(serverName string, toolName string) string {
	serverName = sanitizeToolNamePart(serverName, 20)
	toolName = sanitizeToolNamePart(toolName, 36)
	if serverName == "" {
		serverName = "server"
	}
	if toolName == "" {
		toolName = "tool"
	}
	return "mcp_" + serverName + "_" + toolName
}

func sanitizeToolNamePart(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	value = unsupportedToolNameChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_-")
	if maxLength > 0 && len(value) > maxLength {
		value = value[:maxLength]
		value = strings.Trim(value, "_-")
	}
	return value
}

func formatCallResult(result CallResult) string {
	parts := make([]string, 0, len(result.Content)+1)
	for _, item := range result.Content {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "text":
			text := strings.TrimSpace(item.Text)
			if text != "" {
				parts = append(parts, text)
			}
		case "image", "audio":
			parts = append(parts, formatBinaryContent(item))
		case "resource":
			if len(item.Resource) > 0 {
				parts = append(parts, string(item.Resource))
			}
		default:
			if item.Text != "" {
				parts = append(parts, strings.TrimSpace(item.Text))
				continue
			}
			raw, err := json.Marshal(item)
			if err == nil {
				parts = append(parts, string(raw))
			}
		}
	}

	if len(result.StructuredContent) > 0 {
		parts = append(parts, string(result.StructuredContent))
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func formatBinaryContent(item ContentItem) string {
	label := strings.TrimSpace(item.Type)
	if label == "" {
		label = "binary"
	}
	mimeType := strings.TrimSpace(item.MIMEType)
	if mimeType == "" {
		mimeType = "unknown"
	}
	if strings.TrimSpace(item.Data) == "" {
		return fmt.Sprintf("[%s content mime=%s]", label, mimeType)
	}
	return fmt.Sprintf("[%s content mime=%s base64_bytes=%d]", label, mimeType, len(item.Data))
}

func emptyObjectSchema() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
