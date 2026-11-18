package service

import (
	"testing"

	domainmessage "myai/core/domain/message"
	"myai/core/port/repository"
)

func TestUploadOnlyAssetSummaryKeepsIdentityWithoutShortURL(t *testing.T) {
	items := AssetListItems([]repository.AssetRecord{{
		ID: "record-1", AssetID: "asset-upload", BusinessID: "business-upload", Checksum: "sha256:test",
		SessionID: "session-1", FileName: "guide.md", ContentType: "text/markdown", Size: 5, MaxVisits: -1,
	}})
	if len(items) != 1 {
		t.Fatalf("asset summary count = %d", len(items))
	}
	item := items[0]
	if item.ID != "record-1" || item.AssetID != "asset-upload" || item.BusinessID != "business-upload" || item.Checksum != "sha256:test" || item.MaxVisits != -1 || item.ShortURL != "" || item.ShortCode != "" {
		t.Fatalf("upload-only asset summary changed: %#v", item)
	}
}

func TestPlanStepPromptIsOmittedFromVisibleHistory(t *testing.T) {
	records := []repository.MessageRecord{
		{ID: "user", Role: repository.RoleUser, Content: "implement this"},
		{ID: "plan-step", Role: repository.RoleUser, Content: "Execute the approved plan step", SyntheticReason: string(domainmessage.SyntheticReasonPlanStep)},
		{ID: "assistant", Role: repository.RoleAssistant, Content: "done"},
	}
	visible := visibleMessageRecords(records)
	if len(visible) != 2 || visible[0].ID != "user" || visible[1].ID != "assistant" {
		t.Fatalf("plan execution prompt leaked into visible history: %#v", visible)
	}
}
