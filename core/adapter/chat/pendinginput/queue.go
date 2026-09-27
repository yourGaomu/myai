package pendinginput

import (
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	generationport "myai/core/application/chat/generation/port"
	domaingeneration "myai/core/domain/generation"
)

const (
	MaxPendingMessages = 8
	MaxPendingRunes    = 8000
)

type Queue struct {
	mu           sync.Mutex
	messages     map[string][]domaingeneration.PendingTurnInputItem
	acknowledger generationport.PendingInputAcknowledger
}

func NewQueue() *Queue {
	return &Queue{messages: make(map[string][]domaingeneration.PendingTurnInputItem)}
}

var _ generationport.PendingTurnInput = (*Queue)(nil)
var _ generationport.IdentifiedPendingTurnInput = (*Queue)(nil)
var _ generationport.PendingInputConfigurer = (*Queue)(nil)
var _ generationport.PendingInputReleaser = (*Queue)(nil)

func (q *Queue) Enqueue(sessionID, content string) error {
	sessionID = strings.TrimSpace(sessionID)
	content = strings.TrimSpace(content)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if content == "" {
		return errors.New("pending turn input is empty")
	}
	if utf8.RuneCountInString(content) > MaxPendingRunes {
		return errors.New("pending turn input is too long")
	}
	if q == nil {
		return errors.New("pending turn input queue is nil")
	}
	return q.enqueue(sessionID, "", content)
}

func (q *Queue) EnqueueIdentified(sessionID, messageID, content string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("pending message id is empty")
	}
	return q.enqueue(sessionID, messageID, content)
}

func (q *Queue) enqueue(sessionID, messageID, content string) error {
	sessionID = strings.TrimSpace(sessionID)
	content = strings.TrimSpace(content)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if content == "" {
		return errors.New("pending turn input is empty")
	}
	if utf8.RuneCountInString(content) > MaxPendingRunes {
		return errors.New("pending turn input is too long")
	}
	if q == nil {
		return errors.New("pending turn input queue is nil")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range q.messages[sessionID] {
		if messageID != "" && item.ID == messageID {
			return nil
		}
	}
	if len(q.messages[sessionID]) >= MaxPendingMessages {
		return errors.New("pending turn input queue is full")
	}
	q.messages[sessionID] = append(q.messages[sessionID], domaingeneration.PendingTurnInputItem{ID: messageID, Content: content})
	return nil
}

func (q *Queue) Drain(sessionID string) []string {
	if q == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.messages[sessionID]
	delete(q.messages, sessionID)
	if len(items) == 0 {
		return nil
	}
	contents := make([]string, 0, len(items))
	for _, item := range items {
		contents = append(contents, item.Content)
	}
	return contents
}

func (q *Queue) DrainIdentified(sessionID string) []domaingeneration.PendingTurnInputItem {
	if q == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.messages[sessionID]
	delete(q.messages, sessionID)
	if len(items) == 0 {
		return nil
	}
	return append([]domaingeneration.PendingTurnInputItem(nil), items...)
}

// RequeueIdentified restores a claimed batch in its original order. Existing
// IDs are ignored so a retry cannot duplicate a durable agent message.
func (q *Queue) RequeueIdentified(sessionID string, items []domaingeneration.PendingTurnInputItem) error {
	if q == nil {
		return errors.New("pending turn input queue is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if len(items) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	current := q.messages[sessionID]
	seen := make(map[string]struct{}, len(current)+len(items))
	for _, item := range current {
		if item.ID != "" {
			seen[item.ID] = struct{}{}
		}
	}
	restored := make([]domaingeneration.PendingTurnInputItem, 0, len(items)+len(current))
	for _, item := range items {
		item.Content = strings.TrimSpace(item.Content)
		if item.Content == "" {
			return errors.New("pending turn input is empty")
		}
		if utf8.RuneCountInString(item.Content) > MaxPendingRunes {
			return errors.New("pending turn input is too long")
		}
		if item.ID != "" {
			if _, exists := seen[item.ID]; exists {
				continue
			}
			seen[item.ID] = struct{}{}
		}
		restored = append(restored, item)
	}
	if len(restored)+len(current) > MaxPendingMessages {
		return errors.New("pending turn input queue is full")
	}
	restored = append(restored, current...)
	q.messages[sessionID] = restored
	return nil
}

func (q *Queue) SetAcknowledger(acknowledger generationport.PendingInputAcknowledger) {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.acknowledger = acknowledger
	q.mu.Unlock()
}

func (q *Queue) Acknowledge(messageID string) error {
	if q == nil || strings.TrimSpace(messageID) == "" {
		return nil
	}
	q.mu.Lock()
	acknowledger := q.acknowledger
	q.mu.Unlock()
	if acknowledger == nil {
		return nil
	}
	return acknowledger.Acknowledge(messageID)
}

func (q *Queue) HasPending(sessionID string) bool {
	if q == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages[sessionID]) > 0
}
