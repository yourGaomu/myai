package mapper

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"myai/core/adapter/persistence/mongo/agentrun/po"
	domainagentrun "myai/core/domain/agentrun"
)

func TestRunAndEventRoundTrip(t *testing.T) {
	finishedAt := time.Date(2026, 7, 31, 2, 3, 4, 0, time.UTC)
	run := domainagentrun.Run{
		ID: "run-1", RequestID: "request-1", SessionID: "session-1", Kind: domainagentrun.KindPlan,
		Title: "Plan", Reason: "approved", Status: domainagentrun.StatusFailed, CurrentStep: 1,
		TotalSteps: 3, LastSequence: 7, ErrorMessage: "failed", StartedAt: finishedAt.Add(-time.Minute), FinishedAt: &finishedAt,
	}
	if got := RunDomainFromDocument(RunDocumentFromDomain(run)); !reflect.DeepEqual(got, run) {
		t.Fatalf("run round trip mismatch: got %#v want %#v", got, run)
	}
	event := domainagentrun.Event{
		ID: "event-1", RunID: run.ID, SessionID: run.SessionID, Sequence: 7,
		Type: domainagentrun.EventTypeToolResult, Title: "Tool finished", Content: "result",
		ToolName: "read_file", Arguments: "{}", Status: "failed", ErrorCode: "io_error",
		ErrorMessage: "read failed", Truncated: true, CurrentStep: 1, TotalSteps: 3, CreatedAt: finishedAt,
	}
	if got := EventDomainFromDocument(EventDocumentFromDomain(event)); !reflect.DeepEqual(got, event) {
		t.Fatalf("event round trip mismatch: got %#v want %#v", got, event)
	}
}

func TestAnswerDocumentRetainsOptionalHistoryMessageID(t *testing.T) {
	for _, messageID := range []string{"history-row", ""} {
		event := domainagentrun.Event{ID: "event-1", Type: domainagentrun.EventTypeAnswer, Content: "answer", MessageID: messageID}
		encoded, err := bson.Marshal(EventDocumentFromDomain(event))
		if err != nil {
			t.Fatal(err)
		}
		value, lookupErr := bson.Raw(encoded).LookupErr("message_id")
		if messageID == "" && lookupErr == nil || messageID != "" && (lookupErr != nil || value.StringValue() != messageID) {
			t.Fatalf("unexpected optional message_id for %q: %v / %v", messageID, value, lookupErr)
		}
		var document po.EventDocument
		if err := bson.Unmarshal(encoded, &document); err != nil {
			t.Fatal(err)
		}
		if got := EventDomainFromDocument(document); got.MessageID != messageID || got.ID != event.ID {
			t.Fatalf("answer lost history or event identity: %+v", got)
		}
	}
}
