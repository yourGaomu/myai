package service_test

import (
	"reflect"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	chatmapper "myai/core/adapter/persistence/chatmessage/mapper"
	mongomapper "myai/core/adapter/persistence/mongo/mapper"
	"myai/core/adapter/persistence/mongo/po"
	service "myai/core/application/session/load/service"
	"myai/core/contextmgr"
	message "myai/core/domain/message"
	"myai/core/session"
)

func TestReloadKeepsAssistantBoundariesAndMultipleToolCalls(t *testing.T) {
	current := session.NewFromState(session.InitialState{ID: "reload", Model: "test"})
	current.AppendMessages(
		message.Text(message.RoleUser, "question"),
		message.Assistant("same text", " first reasoning ", []message.ToolCall{
			{ID: "a", Type: "function", Name: "read", Arguments: "{}"},
			{ID: "b", Type: "function", Name: "read", Arguments: "{}"},
		}),
		message.Assistant("", " reasoning only ", []message.ToolCall{{ID: "c", Type: "function", Name: "read", Arguments: "{}"}}),
		message.Assistant("same text", " final reasoning ", nil),
	)
	mapper := chatmapper.Mapper{}
	records := mapper.MemoryMessages(current)
	for i, record := range records {
		encoded, err := bson.Marshal(mongomapper.MessageDocumentFromRecord(record))
		if err != nil {
			t.Fatal(err)
		}
		var document po.MessageDocument
		if err := bson.Unmarshal(encoded, &document); err != nil {
			t.Fatal(err)
		}
		records[i] = mongomapper.MessageRecordFromDocument(document)
	}
	// 1. Mongo 的时间精度为毫秒，仍须通过 sequence 与记录 ID 恢复正确顺序。
	sort.Slice(records, func(i, j int) bool {
		if records[i].Sequence != records[j].Sequence {
			return records[i].Sequence < records[j].Sequence
		}
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.Before(records[j].CreatedAt)
		}
		return records[i].ID < records[j].ID
	})
	restored := service.MessagesFromRecords(records)
	if len(restored) != len(current.Messages) || contextmgr.StableMessagesHash(restored) != contextmgr.StableMessagesHash(current.Messages) {
		t.Fatalf("assistant boundaries changed: %+v", restored)
	}
	for i := 1; i < len(restored); i++ {
		if restored[i].ID != current.Messages[i].ID || restored[i].Sequence != current.Messages[i].Sequence {
			t.Fatal("message identity changed")
		}
	}
	current.Messages = restored
	if got := mapper.MemoryMessages(current); !reflect.DeepEqual(got, records) {
		t.Fatalf("second persistence changed parts or record IDs:\ngot=%+v\nwant=%+v", got, records)
	}
}
