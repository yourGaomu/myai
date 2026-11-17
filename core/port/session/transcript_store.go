package session

import (
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

// TranscriptStore owns mutable session state. Returned snapshots and messages
// are detached from the store and may be projected for an individual request.
type TranscriptStore interface {
	Snapshot(sessionID string) (*session.Session, error)
	// AppendMessagesTo appends one batch atomically and returns the fresh session
	// snapshot and the actual messages with their durable identity metadata.
	AppendMessagesTo(sessionID string, messages ...domainmessage.Message) (snapshot *session.Session, appended []domainmessage.Message, err error)
}
