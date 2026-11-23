package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPListWireRemainsCompatible(t *testing.T) {
	legacy, err := json.Marshal(MCPListResultPayload{Servers: []MCPServerSummary{{Name: "fixture", State: "loaded"}}})
	if err != nil || string(legacy) != `{"servers":[{"name":"fixture","state":"loaded","inventory_tool_count":0,"exposed_tool_count":0}]}` {
		t.Fatalf("unexpected legacy list payload: %s (%v)", legacy, err)
	}
	zero := uint64(0)
	updated, err := json.Marshal(MCPListResultPayload{CatalogRevision: &zero, Servers: []MCPServerSummary{{Name: "fixture", State: "unavailable", Reason: "pending_trust"}}})
	if err != nil || !strings.Contains(string(updated), `"catalog_revision":0`) || !strings.Contains(string(updated), `"reason":"pending_trust"`) {
		t.Fatalf("unexpected versioned list payload: %s (%v)", updated, err)
	}
}

func TestMCPDetailRequestValidatesScope(t *testing.T) {
	original, err := NewMessage(TypeMCPDetail, "detail", "user", "device", "", MCPDetailPayload{ServerName: "demo", Cursor: "opaque:1"})
	if err != nil {
		t.Fatal(err)
	}
	want := MCPDetailPayload{ServerName: "demo", Cursor: "opaque:1"}
	if got, err := MCPDetailRequest(original); err != nil || got != want {
		t.Fatalf("unscoped inspection did not decode: %+v (%v)", got, err)
	}
	for _, raw := range []string{`{}`, `null`, `[]`, `{"server_name":" demo"}`, `{"server_name":"demo","session_id":"embedded"}`, `{"server_name":"demo","cursor":42}`, `{"server_name":"demo","cursor":"` + strings.Repeat("x", 1025) + `"}`, `{"server_name":"demo"` + strings.Repeat(" ", mcpDetailMaxRequestPayload) + `}`} {
		bad := original
		bad.Payload = json.RawMessage(raw)
		if _, err := MCPDetailRequest(bad); err == nil {
			t.Fatalf("accepted malformed request of %d bytes", len(raw))
		}
	}
	for _, session := range []string{" current", "ses\nsion", strings.Repeat("z", 513)} {
		bad := original
		bad.SessionID = session
		if _, err := MCPDetailRequest(bad); err == nil {
			t.Fatalf("accepted invalid session %q", session)
		}
	}
	original.RequestID = ""
	if _, err := MCPDetailRequest(original); err == nil {
		t.Fatal("accepted request without an ID")
	}
}

func TestMCPDetailResultValidatesProjectionAndLimits(t *testing.T) {
	tool := MCPToolDescriptor{
		ID: "fixture-id", Name: "read", Title: "Read", Alias: "mcp_demo_read", MetadataTruncated: true,
		Input:  MCPSchemaSummary{State: "fields", Fields: []MCPSchemaField{{Name: "path", Type: "string", Required: true}}},
		Output: MCPSchemaSummary{State: "undeclared"}, Eligibility: "filtered", Reason: "allowlist",
	}
	payload := MCPDetailResultPayload{
		Server:          MCPServerSummary{Name: "demo", State: "loaded", InventoryToolCount: 30, ExposedToolCount: 30},
		CatalogRevision: 12, SessionEvaluated: true, NextCursor: "opaque",
		Tools: []MCPToolDescriptor{tool},
	}
	makeResult := func(p MCPDetailResultPayload) Message {
		t.Helper()
		message, err := NewMessage(TypeMCPDetailResult, "detail", "user", "device", "current", p)
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	message := makeResult(payload)
	got, err := MCPDetailResult(message)
	if err != nil || got.CatalogRevision != payload.CatalogRevision || !got.SessionEvaluated || len(got.Tools) != 1 || !got.Tools[0].Input.Fields[0].Required {
		t.Fatalf("unexpected result: %+v (%v)", got, err)
	}
	for _, raw := range []string{
		strings.Replace(string(message.Payload), `"session_evaluated":true,`, "", 1),
		strings.Replace(string(message.Payload), `"catalog_revision":12,`, "", 1),
		strings.Replace(string(message.Payload), `"input":{`, `"input":{"raw_schema":{"type":"string"},`, 1),
		strings.Replace(string(message.Payload), `"reason":"allowlist"`, `"reason":"arbitrary upstream text"`, 1),
	} {
		bad := message
		bad.Payload = json.RawMessage(raw)
		if _, err := MCPDetailResult(bad); err == nil {
			t.Fatalf("accepted invalid result: %s", raw)
		}
	}
	bad := message
	bad.SessionID = ""
	if _, err := MCPDetailResult(bad); err == nil {
		t.Fatal("accepted evaluated scope without a session")
	}
	payload.Tools = make([]MCPToolDescriptor, MCPDetailMaxTools)
	for i := range payload.Tools {
		payload.Tools[i] = tool
	}
	if _, err := MCPDetailResult(makeResult(payload)); err != nil {
		t.Fatalf("rejected maximal page count: %v", err)
	}
	payload.Tools = append(payload.Tools, tool)
	if _, err := MCPDetailResult(makeResult(payload)); err == nil {
		t.Fatal("accepted page over tool limit")
	}
	payload.Tools = []MCPToolDescriptor{tool}
	sizeWithoutDescription := len(makeResult(payload).Payload)
	payload.Tools[0].Description = strings.Repeat("x", MCPDetailResponseMaxBytes-sizeWithoutDescription-len(`,"description":""`))
	maximal := makeResult(payload)
	if len(maximal.Payload) != MCPDetailResponseMaxBytes {
		t.Fatalf("bad fixture size: %d", len(maximal.Payload))
	}
	if _, err := MCPDetailResult(maximal); err != nil {
		t.Fatalf("rejected 64 KiB payload: %v", err)
	}
	payload.Tools[0].Description += "x"
	if _, err := MCPDetailResult(makeResult(payload)); err == nil {
		t.Fatal("accepted payload over 64 KiB")
	}
}
