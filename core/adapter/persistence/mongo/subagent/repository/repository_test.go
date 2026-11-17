package repository

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"myai/core/adapter/persistence/mongo/subagent/mapper"
	"myai/core/adapter/persistence/mongo/subagent/po"
	domainsubagent "myai/core/domain/subagent"
)

func TestReplacementUpdateExcludesImmutableIDAndUnsetsOmittedFields(t *testing.T) {
	update, err := replacementUpdate(po.RunDocument{
		ID: "run-1", TaskID: "task-1", Status: "running",
	}, "result", "error_message", "started_at", "completed_at")
	if err != nil {
		t.Fatal(err)
	}

	setValues := update["$set"].(bson.M)
	if _, exists := setValues["_id"]; exists {
		t.Fatalf("immutable _id must not be included in $set: %#v", setValues)
	}
	if setValues["task_id"] != "task-1" || setValues["status"] != "running" {
		t.Fatalf("unexpected $set values: %#v", setValues)
	}

	unsetValues := update["$unset"].(bson.M)
	for _, field := range []string{"result", "error_message", "started_at", "completed_at"} {
		if _, exists := unsetValues[field]; !exists {
			t.Fatalf("expected %q in $unset: %#v", field, unsetValues)
		}
	}
}

func TestReplacementUpdateKeepsPresentOptionalFieldsOutOfUnset(t *testing.T) {
	update, err := replacementUpdate(po.DefinitionDocument{
		ID: "definition-1", Name: "reviewer", ModelID: "model-1", AllowedTools: []string{"read_file"},
	}, "description", "model_id", "allowed_tools", "deleted_at")
	if err != nil {
		t.Fatal(err)
	}

	setValues := update["$set"].(bson.M)
	if setValues["model_id"] != "model-1" {
		t.Fatalf("expected model_id in $set: %#v", setValues)
	}
	unsetValues := update["$unset"].(bson.M)
	if _, exists := unsetValues["model_id"]; exists {
		t.Fatalf("present model_id must not be unset: %#v", unsetValues)
	}
	if _, exists := unsetValues["allowed_tools"]; exists {
		t.Fatalf("present allowed_tools must not be unset: %#v", unsetValues)
	}
}

func completionFindResponse(t *testing.T, collection string, document any) bson.D {
	t.Helper()
	encoded, err := bson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{
		{Key: "id", Value: int64(0)}, {Key: "ns", Value: "fake_subagent_test." + collection},
		{Key: "firstBatch", Value: bson.A{bson.Raw(encoded)}},
	}}}
}

func completionUpdateResponse() bson.D {
	return bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}}
}

func TestCompleteParentCompletionWritesUnreadInSameMongoTransaction(t *testing.T) {
	for _, kind := range []string{"matching result", "new run", "new completion", "new result", "canceled", "already delivered", "task write failure"} {
		t.Run(kind, func(t *testing.T) {
			now := time.Now().UTC()
			expires := now.Add(time.Minute)
			task := domainsubagent.Task{ID: "task-1", CurrentRunID: "run-1", ParentSessionID: "parent-1",
				Status: domainsubagent.TaskStatusSucceeded, Result: "full saved result", Unread: true, UpdatedAt: now, CompletedAt: &now}
			content, err := domainsubagent.CompletionMessageContent(task)
			if err != nil {
				t.Fatal(err)
			}
			message := domainsubagent.AgentMessage{ID: domainsubagent.AgentResultMessageID(task.ID, task.CurrentRunID), SourceTaskID: task.ID,
				RecipientAgentID: task.ParentSessionID, Kind: domainsubagent.AgentMessageKindTaskResult, Trigger: domainsubagent.AgentMessageTriggerTurn,
				Content: content, Status: domainsubagent.AgentMessageDelivering, ClaimOwnerID: "owner-1", ClaimExpiresAt: &expires, CreatedAt: now}
			switch kind {
			case "new run":
				task.CurrentRunID = "run-2"
			case "new completion":
				completed := now.Add(time.Second)
				task.CompletedAt = &completed
			case "new result":
				task.Result = "a newer completed result"
			case "canceled":
				task.Status, task.Unread = domainsubagent.TaskStatusCanceled, false
			case "already delivered":
				message.Status = domainsubagent.AgentMessageDelivered
			}
			responses := []bson.D{completionFindResponse(t, messagesCollection, mapper.AgentMessageDocumentFromDomain(message))}
			if kind != "already delivered" {
				responses = append(responses, completionUpdateResponse(), completionFindResponse(t, tasksCollection, mapper.TaskDocumentFromDomain(task)))
				if kind == "matching result" {
					responses = append(responses, completionUpdateResponse())
				}
				if kind == "task write failure" {
					responses = append(responses, bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: 121}, {Key: "errmsg", Value: "fake task write failure"}})
				}
			}
			responses = append(responses, bson.D{{Key: "ok", Value: 1}}) // commit or abort
			deployment := drivertest.NewMockDeployment(responses...)
			type command struct {
				name     string
				document bson.Raw
			}
			var commands []command
			clientOptions := options.Client().SetRetryWrites(false).SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, event *event.CommandStartedEvent) {
				commands = append(commands, command{event.CommandName, append(bson.Raw(nil), event.Command...)})
			}})
			clientOptions.Deployment = deployment
			client, err := gomongo.Connect(clientOptions)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
			repository := New(client, "fake_subagent_test")
			err = repository.CompleteParentCompletion(context.Background(), message.ID, "owner-1", now.Add(time.Millisecond))
			if kind == "task write failure" {
				if err == nil {
					t.Fatal("expected task write failure to abort ACK")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(commands) == 0 {
				t.Fatal("ACK did not reach the fake Mongo deployment")
			}
			transaction, ok := commands[0].document.Lookup("txnNumber").Int64OK()
			if !ok || !commands[0].document.Lookup("startTransaction").Boolean() {
				t.Fatal("ACK did not begin a transaction")
			}
			taskWrites, messageWrites := 0, 0
			for _, command := range commands {
				if value, ok := command.document.Lookup("txnNumber").Int64OK(); !ok || value != transaction {
					t.Fatal("ACK projections used different transactions")
				}
				if command.document.Lookup("autocommit").Boolean() {
					t.Fatal("ACK write escaped transaction")
				}
				if command.name != "update" {
					continue
				}
				collection := command.document.Lookup("update").StringValue()
				if collection == messagesCollection {
					messageWrites++
					continue
				}
				if collection != tasksCollection {
					t.Fatal("ACK wrote an unrelated collection")
				}
				taskWrites++
				updates, _ := command.document.Lookup("updates").Array().Values()
				update := updates[0].Document()
				filter := update.Lookup("q").Document()
				set := update.Lookup("u").Document().Lookup("$set").Document()
				if filter.Lookup("_id").StringValue() != task.ID || filter.Lookup("current_run_id").StringValue() != task.CurrentRunID || !filter.Lookup("unread").Boolean() {
					t.Fatal("persisted unread update is missing its task/run/read guard")
				}
				if set.Lookup("unread").Boolean() || set.Lookup("updated_at").Type == 0 || set.Lookup("result").Type != 0 {
					t.Fatal("ACK did not update only the read projection")
				}
			}
			wantTaskWrites := 0
			if kind == "matching result" || kind == "task write failure" {
				wantTaskWrites = 1
			}
			if taskWrites != wantTaskWrites {
				t.Fatalf("task writes = %d, want %d", taskWrites, wantTaskWrites)
			}
			if kind == "already delivered" && messageWrites != 0 {
				t.Fatal("duplicate ACK rewrote delivered state")
			}
			last := commands[len(commands)-1].name
			if kind == "task write failure" {
				if last != "abortTransaction" {
					t.Fatal("failed projection update did not roll message delivery back")
				}
				// Replay after the aborted transaction must still be able to
				// commit both delivery and read projections using the same ID.
				deployment.AddResponses(completionFindResponse(t, messagesCollection, mapper.AgentMessageDocumentFromDomain(message)), completionUpdateResponse(),
					completionFindResponse(t, tasksCollection, mapper.TaskDocumentFromDomain(task)), completionUpdateResponse(), bson.D{{Key: "ok", Value: 1}})
				if err := repository.CompleteParentCompletion(context.Background(), message.ID, "owner-1", now.Add(2*time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				if commands[len(commands)-1].name != "commitTransaction" {
					t.Fatal("retried ACK did not commit")
				}
			} else if last != "commitTransaction" {
				t.Fatal("successful ACK did not commit")
			}
		})
	}
}
