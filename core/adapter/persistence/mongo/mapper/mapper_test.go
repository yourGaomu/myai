package mapper

import (
	"reflect"
	"testing"
	"time"

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

	roundTrip := SessionRecordFromDocument(SessionDocumentFromRecord(record))
	if !reflect.DeepEqual(roundTrip, record) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", roundTrip, record)
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
		ID: "asset-1", SessionID: "session-1", LocalPath: "out.txt",
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
