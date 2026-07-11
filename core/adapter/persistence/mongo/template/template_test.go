package template

import (
	"context"
	"errors"
	"testing"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestTemplateImplementsOperations(t *testing.T) {
	var _ Operations = (*Template)(nil)
}

func TestTemplateRejectsNilDatabase(t *testing.T) {
	template := New(nil)
	if err := template.FindOne(context.Background(), "sessions", map[string]string{}, &struct{}{}); err == nil {
		t.Fatal("expected nil database error")
	}
	if _, err := template.Count(context.Background(), "sessions", map[string]string{}); err == nil {
		t.Fatal("expected nil database error")
	}
}

func TestTranslateErrorMapsNotFound(t *testing.T) {
	if err := translateError(gomongo.ErrNoDocuments); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected mongo not found, got %v", err)
	}
	expected := errors.New("database failed")
	if err := translateError(expected); !errors.Is(err, expected) {
		t.Fatalf("expected original error, got %v", err)
	}
}
