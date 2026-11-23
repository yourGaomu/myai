package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

const (
	TypeMCPDetail       MessageType = "mcp_detail"
	TypeMCPDetailResult MessageType = "mcp_detail_result"

	MCPDetailRequestMaxJSONBytes = 8 * 1024  // entire request envelope
	MCPDetailResponseMaxBytes    = 64 * 1024 // encoded response payload
	MCPDetailMaxTools            = 25

	MCPErrorCatalogChanged = "mcp_catalog_changed"

	mcpDetailMaxRequestPayload = 4 * 1024
	mcpDetailMaxServerName     = 1024
	mcpDetailMaxCursor         = 1024
)

var (
	errInvalidMCPDetailRequest = errors.New("invalid mcp detail request")
	errInvalidMCPDetailResult  = errors.New("invalid mcp detail result")
)

type MCPDetailPayload struct {
	ServerName string `json:"server_name"`
	Cursor     string `json:"cursor,omitempty"`
}

type MCPDetailResultPayload struct {
	Server           MCPServerSummary    `json:"server"`
	CatalogRevision  uint64              `json:"catalog_revision"`
	SessionEvaluated bool                `json:"session_evaluated"`
	Tools            []MCPToolDescriptor `json:"tools"`
	NextCursor       string              `json:"next_cursor,omitempty"`
}

type MCPToolDescriptor struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Title             string           `json:"title,omitempty"`
	Description       string           `json:"description,omitempty"`
	Alias             string           `json:"alias,omitempty"`
	HostOnly          bool             `json:"host_only"`
	MetadataTruncated bool             `json:"metadata_truncated,omitempty"`
	Input             MCPSchemaSummary `json:"input"`
	Output            MCPSchemaSummary `json:"output"`
	Eligibility       string           `json:"eligibility"`
	Reason            string           `json:"reason,omitempty"`
}

type MCPSchemaSummary struct {
	State     string           `json:"state"`
	Fields    []MCPSchemaField `json:"fields,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
}

type MCPSchemaField struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required"`
}

type MCPDetailErrorPayload struct {
	ErrorPayload
	RequestType MessageType `json:"request_type"`
	ServerName  string      `json:"server_name,omitempty"`
}

// MCPDetailRequest validates only the transport shape. The catalog validates
// the cursor against its server and revision.
func MCPDetailRequest(message Message) (MCPDetailPayload, error) {
	var payload MCPDetailPayload
	if message.Type != TypeMCPDetail || !mcpScopeText(message.RequestID, 256) || message.RequestID == "" ||
		!mcpScopeText(message.SessionID, 512) || len(message.Payload) > mcpDetailMaxRequestPayload ||
		!decodeMCPDetailPayload(message.Payload, &payload) || payload.ServerName == "" ||
		!mcpScopeText(payload.ServerName, mcpDetailMaxServerName) || !mcpScopeText(payload.Cursor, mcpDetailMaxCursor) {
		return MCPDetailPayload{}, errInvalidMCPDetailRequest
	}
	return payload, nil
}

func MCPDetailResult(message Message) (MCPDetailResultPayload, error) {
	var wire struct {
		MCPDetailResultPayload
		CatalogRevision  *uint64 `json:"catalog_revision"`
		SessionEvaluated *bool   `json:"session_evaluated"`
	}
	if message.Type != TypeMCPDetailResult || len(message.Payload) > MCPDetailResponseMaxBytes ||
		!decodeMCPDetailPayload(message.Payload, &wire) || wire.CatalogRevision == nil || wire.SessionEvaluated == nil {
		return MCPDetailResultPayload{}, errInvalidMCPDetailResult
	}
	payload := wire.MCPDetailResultPayload
	payload.CatalogRevision = *wire.CatalogRevision
	payload.SessionEvaluated = *wire.SessionEvaluated
	if !validMCPDetailServer(payload.Server) || payload.SessionEvaluated && message.SessionID == "" ||
		payload.Tools == nil || len(payload.Tools) > MCPDetailMaxTools || !mcpScopeText(payload.NextCursor, mcpDetailMaxCursor) {
		return MCPDetailResultPayload{}, errInvalidMCPDetailResult
	}
	for _, tool := range payload.Tools {
		if tool.ID == "" || tool.Name == "" || !validMCPSchemaSummary(tool.Input) || !validMCPSchemaSummary(tool.Output) ||
			!validMCPToolEligibility(tool, payload.SessionEvaluated) {
			return MCPDetailResultPayload{}, errInvalidMCPDetailResult
		}
	}
	return payload, nil
}

func MCPDetailError(message Message) (MCPDetailErrorPayload, error) {
	var payload MCPDetailErrorPayload
	if message.Type != TypeError || len(message.Payload) > MCPDetailResponseMaxBytes ||
		!decodeMCPDetailPayload(message.Payload, &payload) || payload.RequestType != TypeMCPDetail ||
		!mcpScopeText(payload.ServerName, mcpDetailMaxServerName) || strings.TrimSpace(payload.Message) == "" ||
		(payload.Code != "" && payload.Code != MCPErrorCatalogChanged) {
		return MCPDetailErrorPayload{}, errors.New("invalid mcp detail error")
	}
	return payload, nil
}

func mcpScopeText(value string, maxBytes int) bool {
	return len(value) <= maxBytes && value == strings.TrimSpace(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

// A detail payload is a bounded, projected object. Reject additional members so
// Relay cannot forward accidentally embedded raw schemas or other MCP data.
func decodeMCPDetailPayload(raw json.RawMessage, dst any) bool {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 || data[0] != '{' || !json.Valid(data) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst) == nil
}

func validMCPDetailServer(server MCPServerSummary) bool {
	if server.Name == "" || !mcpScopeText(server.Name, mcpDetailMaxServerName) ||
		server.ExposedToolCount < 0 || server.InventoryToolCount < 0 {
		return false
	}
	switch server.State {
	case "loaded", "disabled":
		return server.Reason == ""
	case "unavailable":
		switch server.Reason {
		case "", "pending_trust", "start_failed", "discovery_failed":
			return true
		}
	}
	return false
}

func validMCPSchemaSummary(summary MCPSchemaSummary) bool {
	switch summary.State {
	case "undeclared", "empty", "unsupported":
		return len(summary.Fields) == 0
	case "fields":
		return len(summary.Fields) > 0
	default:
		return false
	}
}

func validMCPToolEligibility(tool MCPToolDescriptor, evaluated bool) bool {
	if tool.HostOnly {
		return tool.Eligibility == "host_only" && tool.Reason == ""
	}
	if !evaluated {
		return tool.Eligibility == "not_evaluated" && tool.Reason == ""
	}
	switch tool.Eligibility {
	case "available":
		return tool.Reason == ""
	case "filtered":
		switch tool.Reason {
		case "agent_mode", "permission_mode", "permission_ceiling", "allowlist", "not_registered":
			return true
		}
	}
	return false
}
