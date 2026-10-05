package repository

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func TestIndexMigrationPreservesCurrentAndOperatorIndexes(t *testing.T) {
	keys, err := bson.Marshal(bson.D{{Key: "session_id", Value: 1}, {Key: "source_id", Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{messageSourceIndexName, "operator_index", "_id_"} {
		if shouldRemoveMessageSourceIndex(name, keys) {
			t.Fatalf("would delete %s on startup", name)
		}
	}
	if !shouldRemoveMessageSourceIndex("session_id_1_source_id_1", keys) {
		t.Fatal("legacy index was not targeted")
	}
}
