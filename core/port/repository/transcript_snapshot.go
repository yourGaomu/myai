package repository

// TranscriptSnapshot is the complete persisted state after regeneration has
// trimmed the previous assistant/tool tail from a Session.
type TranscriptSnapshot struct {
	Session  SessionRecord
	Messages []MessageRecord
}
