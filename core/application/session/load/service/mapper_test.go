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
	"myai/core/port/model"
	"myai/core/port/repository"
	"myai/core/session"
)

func TestPrivateRequestMetadataSurvivesDurableWriteReloadAndRewrite(t *testing.T) {
	current := session.NewFromState(session.InitialState{ID: "private", Model: "test"})
	runtime := message.RuntimeInstruction("skill text")
	runtime.SkillPromptTokens = 73
	user := message.Text(message.RoleUser, "question")
	user.ID = "user"
	toolOnly := message.ToolCallMessage([]message.ToolCall{
		{ID: "a", Type: "function", Name: "read_asset", Arguments: "{}"},
		{ID: "b", Type: "function", Name: "read_asset", Arguments: "{}"},
	})
	toolOnly.ID = "tool-sampling"
	toolOnly.ImageReceipts = []message.ImageReceipt{{Version: 1, AssetID: "user-image", SHA256: "user-sha", MIMEType: "image/png", SourceMessageID: "user", AnchorMessageID: "user"}}
	toolOnly.RequestContext = &message.RequestContext{Model: "test", WindowTokens: 8000}
	firstResult := message.ToolResultMessage(message.ToolResult{ToolCallID: "a", Name: "read_asset", Content: "first result"})
	firstResult.ID = "result-a"
	lastResult := message.ToolResultMessage(message.ToolResult{ToolCallID: "b", Name: "read_asset", Content: "last result"})
	lastResult.ID = "result-b"
	final := message.Assistant("answer", "reasoning", nil)
	final.ID = "final"
	final.ImageReceipts = append([]message.ImageReceipt(nil), toolOnly.ImageReceipts...)
	final.ImageReceipts = append(final.ImageReceipts, message.ImageReceipt{
		Version: 1, AssetID: "tool-image", SHA256: "tool-sha", MIMEType: "image/jpeg", SourceMessageID: "result-a", AnchorMessageID: "result-b", ToolCallID: "a",
	})
	final.RequestContext = &message.RequestContext{Model: "test", WindowTokens: 8000, MessageTokens: 100, SystemTokens: 21, SkillTokens: 73, MCPToolTokens: 7, OtherToolTokens: 3, OtherTokens: 0, OutputReserveTokens: 500, SafetyReserveTokens: 100}
	current.AppendMessages(runtime, user, toolOnly, firstResult, lastResult, final)

	mapper := chatmapper.Mapper{}
	records := mapper.MemoryMessages(current)
	if len(records) != 7 || records[0].SkillPromptTokens != 73 || records[2].Role != repository.RoleToolCall ||
		records[2].ID != "tool-sampling:000000" || !reflect.DeepEqual(records[2].ImageReceipts, toolOnly.ImageReceipts) ||
		records[3].ID != "tool-sampling:000001" || records[3].ImageReceipts != nil || records[3].RequestContext != nil {
		t.Fatalf("unexpected flattened rows: %+v", records)
	}
	if durable := mapper.AssistantTurn(current.ID, model.ChatResult{Message: &current.Messages[6]}, final.CreatedAt); len(durable) != 1 || !reflect.DeepEqual(durable[0], records[6]) {
		t.Fatalf("durable final reply differs from rewrite row: %+v", durable)
	}
	for i, record := range records {
		encoded, err := bson.Marshal(mongomapper.MessageDocumentFromRecord(record))
		if err != nil {
			t.Fatal(err)
		}
		raw := bson.Raw(encoded)
		if i == 1 && (raw.Lookup("image_receipts").Type != 0 || raw.Lookup("request_context").Type != 0 || raw.Lookup("skill_prompt_tokens").Type != 0) {
			t.Fatalf("BSON legacy row includes zero metadata: %s", raw)
		}
		if i == 6 && raw.Lookup("request_context", "other_tokens").Type == 0 {
			t.Fatalf("request context dropped a required zero category: %s", raw)
		}
		var document po.MessageDocument
		if err := bson.Unmarshal(encoded, &document); err != nil {
			t.Fatal(err)
		}
		records[i] = mongomapper.MessageRecordFromDocument(document)
	}

	restored := service.MessagesFromRecords(records)
	if len(restored) != len(current.Messages) || restored[1].SkillPromptTokens != runtime.SkillPromptTokens ||
		!reflect.DeepEqual(restored[3].ImageReceipts, toolOnly.ImageReceipts) || !reflect.DeepEqual(restored[3].RequestContext, toolOnly.RequestContext) ||
		!reflect.DeepEqual(restored[6].ImageReceipts, final.ImageReceipts) || !reflect.DeepEqual(restored[6].RequestContext, final.RequestContext) ||
		restored[3].ID != "tool-sampling" || len(restored[3].Parts) != 2 {
		t.Fatalf("reload lost private metadata or changed transcript shape: %+v", restored)
	}
	withoutMetadata := message.CloneAll(restored)
	for i := range withoutMetadata {
		withoutMetadata[i].ImageReceipts, withoutMetadata[i].RequestContext, withoutMetadata[i].SkillPromptTokens = nil, nil, 0
	}
	if contextmgr.CompactionSourceHash(restored, len(restored)) != contextmgr.CompactionSourceHash(withoutMetadata, len(withoutMetadata)) {
		t.Fatal("private storage metadata changed the compaction source hash")
	}
	current.Messages = restored
	if rewritten := mapper.MemoryMessages(current); !reflect.DeepEqual(rewritten, records) {
		t.Fatalf("rewrite changed persisted rows: got %+v, want %+v", rewritten, records)
	}
	restored[6].ImageReceipts[0].SHA256 = "changed"
	restored[6].RequestContext.Model = "changed"
	if records[6].ImageReceipts[0].SHA256 != "user-sha" || records[6].RequestContext.Model != "test" {
		t.Fatal("loaded message aliases its repository record")
	}
}

func TestReloadKeepsAssistantBoundariesAndMultipleToolCalls(t *testing.T) {
	current := session.NewFromState(session.InitialState{ID: "reload", Model: "test"})
	final := message.Assistant("same text", " final reasoning ", nil)
	final.AssistantUsage = message.AssistantUsage{
		PromptTokens: 12, CompletionTokens: 3, TotalTokens: 15, ReasoningTokens: 2,
		Available: true, PromptCacheReported: true, PromptCacheIncomplete: true,
	}
	current.AppendMessages(
		message.Text(message.RoleUser, "question"),
		message.Assistant("same text", " first reasoning ", []message.ToolCall{
			{ID: "a", Type: "function", Name: "read", Arguments: "{}"},
			{ID: "b", Type: "function", Name: "read", Arguments: "{}"},
		}),
		message.Assistant("", " reasoning only ", []message.ToolCall{{ID: "c", Type: "function", Name: "read", Arguments: "{}"}}),
		final,
	)
	mapper := chatmapper.Mapper{}
	records := mapper.MemoryMessages(current)
	if last := records[len(records)-1]; !last.PromptCacheReported || !last.PromptCacheIncomplete || last.PromptCachedTokens != 0 {
		t.Fatalf("memory history lost the partial zero count: %+v", last)
	}
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
	if got := restored[len(restored)-1].AssistantUsage; got != final.AssistantUsage {
		t.Fatalf("reloaded usage = %+v, want %+v", got, final.AssistantUsage)
	}
	withoutUsage := append([]message.Message(nil), restored...)
	withoutUsage[len(withoutUsage)-1].AssistantUsage = message.AssistantUsage{}
	if contextmgr.StableMessagesHash(restored) != contextmgr.StableMessagesHash(withoutUsage) {
		t.Fatal("assistant usage changed the stable history hash")
	}
	current.Messages = restored
	if got := mapper.MemoryMessages(current); !reflect.DeepEqual(got, records) {
		t.Fatalf("second persistence changed parts or record IDs:\ngot=%+v\nwant=%+v", got, records)
	}
}
