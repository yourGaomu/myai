package outbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSafeErrorDoesNotExposeBackendPayload(t *testing.T) {
	const privateText = "fixture-private-body"
	original := fmt.Errorf("%s: %w", privateText, context.DeadlineExceeded)
	err := SafeError("save records", original)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("safe error must preserve cancellation classification")
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("unsafe backend error must not be available through Unwrap")
	}
	for _, format := range []string{"%s", "%v", "%+v", "%#v", "%q"} {
		if text := fmt.Sprintf(format, err); strings.Contains(text, privateText) {
			t.Fatalf("format %s exposed backend payload", format)
		}
	}
	if SafeError("save records", nil) != nil {
		t.Fatal("successful write must not return an error")
	}
}
