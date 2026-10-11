package mapper

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"myai/core/adapter/persistence/mongo/subagent/po"
	domainsubagent "myai/core/domain/subagent"
	domaintool "myai/core/domain/tool"
	subagentport "myai/core/port/subagent"
	"testing"
	"time"
)

func TestEventIdentityAndChineseAssetSurviveBSON(t *testing.T) {
	expires := time.Now().UTC().Truncate(time.Millisecond)
	event := subagentport.TaskEvent{
		Sequence: 5, Kind: subagentport.TaskEventKindAsset, RunID: "run", ToolCallID: "tool",
		Task:   domainsubagent.Task{ID: "task", ParentSessionID: "parent", ParentToolCallID: "spawn-call", ChildSessionID: "child", CurrentRunID: "run"},
		Assets: []domaintool.SharedAsset{{ID: "asset", SessionID: "child", ToolCallID: "tool", FileName: "实时预览.png", ContentType: "image/png", ShortURL: "https://example.com/a", ExpiresAt: &expires}},
	}
	encoded, err := bson.Marshal(TaskEventDocumentFromDomain(event))
	if err != nil {
		t.Fatal(err)
	}
	var document po.TaskEventDocument
	if err := bson.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	got := TaskEventDomainFromDocument(document)
	if got.Task.ParentToolCallID != "spawn-call" || got.Task.ChildSessionID != "child" || got.ToolCallID != "tool" || len(got.Assets) != 1 || got.Assets[0].ID != "asset" || got.Assets[0].FileName != "实时预览.png" || !got.Assets[0].ExpiresAt.Equal(expires) {
		t.Fatalf("round trip: %#v", got)
	}
}
