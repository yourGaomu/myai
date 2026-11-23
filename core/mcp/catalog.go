package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"myai/core/tool"
	tooldef "myai/core/tool/tool"
)

const (
	ServerPendingTrust    = "pending_trust"
	ServerStartFailed     = "start_failed"
	ServerDiscoveryFailed = "discovery_failed"

	catalogPageSize      = 25
	catalogPageToolBytes = 60 * 1024 // Leave room for the response envelope and service metadata.
	catalogSchemaBytes   = 64 * 1024
	catalogSchemaFields  = 16
)

var (
	ErrCatalogChanged        = errors.New("mcp catalog changed")
	ErrCatalogServerNotFound = errors.New("mcp catalog server not found")
	ErrCatalogInvalidCursor  = errors.New("invalid mcp catalog cursor")
)

type SchemaField struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required"`
}

type SchemaSummary struct {
	State     string        `json:"state"`
	Fields    []SchemaField `json:"fields,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
}

type ToolDescriptor struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Title             string        `json:"title,omitempty"`
	Description       string        `json:"description,omitempty"`
	Alias             string        `json:"alias,omitempty"`
	HostOnly          bool          `json:"host_only"`
	MetadataTruncated bool          `json:"metadata_truncated,omitempty"`
	Input             SchemaSummary `json:"input"`
	Output            SchemaSummary `json:"output"`
	Eligibility       string        `json:"eligibility"`
	Reason            string        `json:"reason,omitempty"`
}

type ServerDetail struct {
	Server           ServerStatus
	CatalogRevision  uint64
	SessionEvaluated bool
	Tools            []ToolDescriptor
	NextCursor       string
}

type catalogTool struct {
	descriptor ToolDescriptor
	registered *Tool
}

func makeCatalogTool(source string, info ToolInfo, occurrence int, registered *Tool) catalogTool {
	name, nameTruncated := catalogText(info.Name, 256)
	title, titleTruncated := catalogText(info.Title, 256)
	description, descriptionTruncated := catalogText(info.Description, 1024)
	key, _ := json.Marshal([3]string{source, info.Name, strconv.Itoa(occurrence)})
	descriptor := ToolDescriptor{
		ID: catalogHash(key), Name: name, Title: title, Description: description,
		HostOnly: registered == nil, MetadataTruncated: nameTruncated || titleTruncated || descriptionTruncated,
		Input: summarizeSchema(info.InputSchema), Output: summarizeSchema(info.OutputSchema),
	}
	if registered != nil {
		descriptor.Alias = registered.Name()
	}
	return catalogTool{descriptor: descriptor, registered: registered}
}

func catalogHash(key []byte) string {
	digest := sha256.Sum256(key)
	return hex.EncodeToString(digest[:])
}

func catalogText(value string, limit int) (string, bool) {
	value = strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			return ' '
		}
		return r
	}, value)
	if len(value) <= limit {
		return value, false
	}
	for !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit], true
}

func summarizeSchema(raw json.RawMessage) SchemaSummary {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return SchemaSummary{State: "undeclared"}
	}
	if len(raw) > catalogSchemaBytes {
		return SchemaSummary{State: "unsupported", Truncated: true}
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || top == nil {
		return SchemaSummary{State: "unsupported"}
	}
	for _, key := range []string{"$ref", "$dynamicRef", "allOf", "anyOf", "oneOf", "not", "if", "patternProperties"} {
		if _, ok := top[key]; ok {
			return SchemaSummary{State: "unsupported"}
		}
	}
	var properties map[string]json.RawMessage
	if encoded, ok := top["properties"]; ok {
		if json.Unmarshal(encoded, &properties) != nil || properties == nil {
			return SchemaSummary{State: "unsupported"}
		}
	} else if schemaType(top["type"]) != "object" && len(top) != 0 {
		return SchemaSummary{State: "unsupported"}
	}
	if len(properties) == 0 {
		return SchemaSummary{State: "empty"}
	}
	var required []string
	if rawRequired, ok := top["required"]; ok && json.Unmarshal(rawRequired, &required) != nil {
		return SchemaSummary{State: "unsupported"}
	}
	isRequired := make(map[string]bool, len(required))
	for _, name := range required {
		isRequired[name] = true
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	summary := SchemaSummary{State: "fields", Truncated: len(names) > catalogSchemaFields}
	for _, name := range names[:min(len(names), catalogSchemaFields)] {
		var field struct{ Type json.RawMessage }
		_ = json.Unmarshal(properties[name], &field)
		label, truncated := catalogText(name, 96)
		summary.Fields = append(summary.Fields, SchemaField{Name: label, Type: schemaType(field.Type), Required: isRequired[name]})
		summary.Truncated = summary.Truncated || truncated
	}
	return summary
}

func schemaType(raw json.RawMessage) string {
	var single string
	var types []string
	if json.Unmarshal(raw, &single) == nil {
		types = []string{single}
	} else if json.Unmarshal(raw, &types) != nil || len(types) > 7 {
		return ""
	}
	seen := make(map[string]bool, len(types))
	for _, item := range types {
		switch item {
		case "object", "array", "string", "integer", "number", "boolean", "null":
		default:
			return ""
		}
		if seen[item] {
			return ""
		}
		seen[item] = true
	}
	return strings.Join(types, " | ")
}

func catalogOffset(serverName, cursor string, revision uint64) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	version, rest, ok := strings.Cut(cursor, ":")
	server, offset, okOffset := strings.Cut(rest, ":")
	position, err := strconv.Atoi(offset)
	if !ok || !okOffset || err != nil || position <= 0 || server != catalogHash([]byte(serverName)) {
		return 0, ErrCatalogInvalidCursor
	}
	if version != strconv.FormatUint(revision, 10) {
		return 0, ErrCatalogChanged
	}
	return position, nil
}

func (m *Manager) ServerDetail(serverName, cursor string, evaluate func(name string, permission tooldef.Permission) string) (ServerDetail, error) {
	// Read manager and winning registry entries under the same publication lock.
	// Candidate discovery happens outside this lock; no inspection pins clients.
	for {
		m.mu.Lock()
		registry := m.registry
		m.mu.Unlock()
		release := registry.BeginReload()
		m.mu.Lock()
		if m.registry != registry {
			m.mu.Unlock()
			release()
			continue
		}
		result, entries, offset, total, err := m.catalogPageLocked(serverName, cursor, registry)
		m.mu.Unlock()
		release()
		if err != nil {
			return ServerDetail{}, err
		}
		result.SessionEvaluated = evaluate != nil
		result.Tools = make([]ToolDescriptor, 0, len(entries))
		budget := 0
		for _, entry := range entries {
			descriptor := entry.descriptor
			switch {
			case descriptor.HostOnly:
				descriptor.Eligibility = "host_only"
			case evaluate == nil:
				descriptor.Eligibility = "not_evaluated"
			default:
				if entry.registered == nil {
					descriptor.Reason = "not_registered"
				} else {
					descriptor.Reason = evaluate(descriptor.Alias, entry.registered.Permission())
				}
				descriptor.Eligibility = "available"
				if descriptor.Reason != "" {
					descriptor.Eligibility = "filtered"
				}
			}
			encoded, _ := json.Marshal(descriptor)
			if budget+len(encoded)+1 > catalogPageToolBytes && len(result.Tools) != 0 {
				break
			}
			budget += len(encoded) + 1
			descriptor.Input.Fields = append([]SchemaField(nil), descriptor.Input.Fields...)
			descriptor.Output.Fields = append([]SchemaField(nil), descriptor.Output.Fields...)
			result.Tools = append(result.Tools, descriptor)
		}
		if offset+len(result.Tools) < total {
			result.NextCursor = strconv.FormatUint(result.CatalogRevision, 10) + ":" + catalogHash([]byte(serverName)) + ":" + strconv.Itoa(offset+len(result.Tools))
		}
		return result, nil
	}
}

func (m *Manager) catalogPageLocked(serverName, cursor string, registry *tool.RegisterTools) (ServerDetail, []catalogTool, int, int, error) {
	offset, err := catalogOffset(serverName, cursor, m.revision)
	if err != nil {
		return ServerDetail{}, nil, 0, 0, err
	}
	for _, server := range m.config.Servers {
		if strings.TrimSpace(server.Name) != serverName {
			continue
		}
		result := ServerDetail{Server: m.serverStatusLocked(server), CatalogRevision: m.revision}
		runtime := m.servers[serverName]
		if runtime == nil && offset == 0 {
			return result, nil, 0, 0, nil
		}
		if runtime == nil || offset != 0 && offset >= len(runtime.catalog) {
			return ServerDetail{}, nil, 0, 0, ErrCatalogInvalidCursor
		}
		total := len(runtime.catalog)
		entries := append([]catalogTool(nil), runtime.catalog[offset:offset+min(total-offset, catalogPageSize)]...)
		for i, entry := range entries {
			if entry.registered != nil {
				if active, _ := registry.GetTool(entry.descriptor.Alias); active != entry.registered {
					entries[i].registered = nil
				}
			}
		}
		return result, entries, offset, total, nil
	}
	return ServerDetail{}, nil, 0, 0, ErrCatalogServerNotFound
}
