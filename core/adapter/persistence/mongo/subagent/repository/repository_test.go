package repository

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"myai/core/adapter/persistence/mongo/subagent/po"
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
