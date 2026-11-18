package mapper

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"reflect"
	"testing"
	"time"

	compaction "myai/core/domain/compaction"
	generation "myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
	agentplan "myai/core/plan"
	repository "myai/core/port/repository"
	"myai/core/session"
)

func TestSessionDocumentRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 11, 10, 30, 0, 0, time.UTC)
	temperature := 0.2
	maxTokens := 2048
	record := repository.SessionRecord{
		ID:                   "session-1",
		Kind:                 string(session.KindSubagent),
		ParentSessionID:      "parent-session-1",
		ParentTaskID:         "task-1",
		AgentDefinitionID:    "code-reviewer",
		AgentDefinitionVer:   3,
		SystemInstruction:    "Review the isolated workspace.",
		AllowedTools:         []string{"read_file", "search_files"},
		EnforceToolAllowlist: true,
		WorkspaceRoot:        "C:/snapshots/task-1",
		WorkspaceSandboxID:   "sandbox-1",
		MaxToolRounds:        6,
		Model:                "gpt-5",
		AgentMode:            "plan",
		PermissionMode:       "ask",
		ContextWindowK:       128,
		Summary:              "summary",
		CompactedMessages:    3,
		CompactionSourceHash: "source-hash",
		CompactedAt:          &now,
		Title:                "title",
		Usage: &repository.TokenUsageRecord{
			PromptTokens:       10,
			CompletionTokens:   5,
			TotalTokens:        15,
			PromptCachedTokens: 4,
			Available:          true,
		},
		CurrentPlan: &agentplan.Plan{
			ID:        "plan-1",
			SessionID: "session-1",
			Goal:      "ship it",
			Status:    agentplan.StatusRunning,
			Steps: []agentplan.Step{{
				ID:     "step-1",
				Order:  1,
				Title:  "Inspect",
				Status: agentplan.StepStatusDone,
			}},
			CreatedAt: now,
			UpdatedAt: now,
		},
		RAGSettings:        session.DefaultRAGSettings(),
		GenerationSettings: generation.Settings{Temperature: &temperature, MaxOutputTokens: &maxTokens},
		StyleInstruction:   "Answer in concise Chinese.",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	checkpoint, err := compaction.NewCheckpoint(`{"current_goal":"继续任务","preferences":[],"constraints":[],"decisions":[],"completed_work":[],"modified_files":[],"tool_verification":[],"problems":[],"open_tasks":[],"next_steps":[],"references":[]}`, 0, 3, "source-hash", now)
	if err != nil {
		t.Fatal(err)
	}
	record.CompactionCheckpoint = &checkpoint

	roundTrip := SessionRecordFromDocument(SessionDocumentFromRecord(record))
	if !reflect.DeepEqual(roundTrip, record) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", roundTrip, record)
	}
}

func TestUploadOnlyAssetPersistsWithoutShortURL(t *testing.T) {
	record := repository.AssetRecord{
		ID: "record-upload", AssetID: "asset-upload", BusinessID: "business-upload", Checksum: "sha256:test",
		SessionID: "session-1", FileName: "guide.md", Size: 5,
	}
	document := AssetDocumentFromRecord(record)
	encoded, err := bson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var fields bson.M
	if err := bson.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["short_url"]; exists {
		t.Fatal("upload-only asset stored an empty public URL")
	}
	if fields["asset_id"] != "asset-upload" || fields["business_id"] != "business-upload" || fields["checksum"] != "sha256:test" {
		t.Fatalf("asset identity not persisted: %#v", fields)
	}
	if got := AssetRecordFromDocument(document); !reflect.DeepEqual(got, record) {
		t.Fatalf("upload-only round trip mismatch: got %#v want %#v", got, record)
	}
}

func TestRecordDocumentMappersRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 11, 10, 30, 0, 0, time.UTC)
	temperature := 0.3
	topP := 0.8
	maxTokens := 4096
	message := repository.MessageRecord{
		ID: "message-1", SessionID: "session-1", Role: repository.RoleTool,
		Content: "done", ToolName: "shell", ToolStatus: "failed", ToolErrorCode: "command_exit_nonzero",
		ToolError: "exit code 1", ToolTruncated: true,
		ToolPromptContent: "bounded", ToolPromptError: "bounded error", ToolPromptTruncated: true,
		TotalTokens: 12, Sequence: now.UnixNano(), CreatedAt: now,
	}
	if got := MessageRecordFromDocument(MessageDocumentFromRecord(message)); !reflect.DeepEqual(got, message) {
		t.Fatalf("message round trip mismatch: got %#v want %#v", got, message)
	}

	asset := repository.AssetRecord{
		ID: "record-1", AssetID: "asset-1", BusinessID: "business-1", Checksum: "sha256:test", MaxVisits: 5,
		SessionID: "session-1", LocalPath: "out.txt",
		ShortURL: "https://example.test/a", Size: 12, CreatedAt: now,
	}
	if got := AssetRecordFromDocument(AssetDocumentFromRecord(asset)); !reflect.DeepEqual(got, asset) {
		t.Fatalf("asset round trip mismatch: got %#v want %#v", got, asset)
	}

	model := domainmodel.Config{
		ID: "gpt-5", Name: "GPT-5", Provider: "openai", BaseURL: "https://api.example.test",
		APIKey: "secret", ModelName: "gpt-5", Enabled: true, IsDefault: true,
		DefaultGenerationSettings: generation.Settings{Temperature: &temperature, TopP: &topP, MaxOutputTokens: &maxTokens},
		CreatedAt:                 now, UpdatedAt: now,
	}
	if got := ModelConfigDomainFromDocument(ModelConfigDocumentFromDomain(model)); !reflect.DeepEqual(got, model) {
		t.Fatalf("model round trip mismatch: got %#v want %#v", got, model)
	}
}
