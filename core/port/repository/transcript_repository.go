package repository

import "context"

// TranscriptRepository replaces messages, removes assets belonging only to
// the trimmed tool calls, and saves the Session as one atomic operation.
type TranscriptRepository interface {
	ReplaceSessionTranscript(ctx context.Context, snapshot TranscriptSnapshot) error
}
