package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	agentrunresult "myai/core/application/agentrun/result"
	sessionresult "myai/core/application/session/result"
	subagentquery "myai/core/application/subagent/query"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
	"myai/core/remote/protocol"
)

type inspectionFacade struct{ SubagentFacade }

func (inspectionFacade) InspectTask(_ context.Context, query subagentquery.InspectTask) (subagentresult.Inspection, error) {
	if query.ParentSessionID != "parent" || query.TaskID != "task" {
		return subagentresult.Inspection{}, subagentport.ErrNotFound
	}
	return subagentresult.Inspection{SessionReady: true, EventSequence: 8, Task: domainsubagent.Task{
		ID: "task", ParentSessionID: "parent", ParentToolCallID: "spawn-call", ChildSessionID: "child", CurrentRunID: "run",
	}}, nil
}
func (inspectionFacade) ListTaskEvents(context.Context, subagentquery.TaskEvents) (subagentresult.EventPage, error) {
	return subagentresult.EventPage{Events: []subagentport.TaskEvent{{Sequence: 8, Kind: subagentport.TaskEventKindAnswer, Content: "实时中文", RunID: "run", Delta: true}}, NextSequence: 8, ThroughSequence: 8}, nil
}

type inspectionChat struct {
	ChatFacade
	t *testing.T
}

func (s inspectionChat) ListSessionMessagesAfter(_ context.Context, id, after string, limit int) ([]sessionresult.MessageListItem, bool, error) {
	if id != "child" || after != "" || limit != 2 {
		s.t.Fatalf("unexpected query: %s %s %d", id, after, limit)
	}
	return []sessionresult.MessageListItem{{ID: "m1", Content: "第一条"}, {ID: "m2", Content: "第二条"}}, false, nil
}
func (s inspectionChat) ListAgentRuns(_ context.Context, id string, _ int) ([]agentrunresult.Snapshot, error) {
	if id != "child" {
		s.t.Fatal("wrong child run scope")
	}
	return nil, nil
}
func (s inspectionChat) ListAssets(_ context.Context, id string, _ int) ([]sessionresult.AssetListItem, error) {
	if id != "child" {
		s.t.Fatal("wrong child asset scope")
	}
	return []sessionresult.AssetListItem{{ID: "asset", SessionID: id, FileName: "图片.png"}}, nil
}

func TestInspectionDispatchUsesChildQueriesWithoutLoadingSession(t *testing.T) {
	a := &Agent{subagentService: inspectionFacade{}, chatService: inspectionChat{t: t}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		a.readLoop(context.Background(), conn, make(chan error, 1))
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeSubagentTaskDetail, RequestID: "detail", SessionID: "parent", Payload: json.RawMessage(`{"task_id":"task","message_limit":1}`)}); err != nil {
		t.Fatal(err)
	}
	var message protocol.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	if message.Type != protocol.TypeSubagentTaskDetailResult {
		t.Fatalf("response: %#v", message)
	}
	detail, err := protocol.DecodePayload[protocol.SubagentTaskDetailResultPayload](message)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.ReadOnly || !detail.HasMoreMessages || detail.NextMessageID != "m1" || detail.Task.ChildSessionID != "child" || detail.Task.ParentToolCallID != "spawn-call" || detail.EventSequence != 8 || detail.Assets[0].FileName != "图片.png" {
		t.Fatalf("detail: %#v", detail)
	}
	if err := conn.WriteJSON(protocol.Message{Type: protocol.TypeSubagentTaskEvents, RequestID: "events", SessionID: "parent", Payload: json.RawMessage(`{"task_id":"task"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	page, err := protocol.DecodePayload[protocol.SubagentTaskEventsResultPayload](message)
	if err != nil || message.Type != protocol.TypeSubagentTaskEventsResult || page.Events[0].Content != "实时中文" || page.NextSequence != 8 {
		t.Fatalf("events: %#v %v", page, err)
	}
}
