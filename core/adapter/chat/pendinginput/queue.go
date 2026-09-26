package pendinginput

import (
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	generationport "myai/core/application/chat/generation/port"
)

const (
	MaxPendingMessages = 8
	MaxPendingRunes    = 8000
)

type Queue struct {
	mu       sync.Mutex
	messages map[string][]string
}

func NewQueue() *Queue {
	return &Queue{messages: make(map[string][]string)}
}

var _ generationport.PendingTurnInput = (*Queue)(nil)

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
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.messages[sessionID]) >= MaxPendingMessages {
		return errors.New("pending turn input queue is full")
	}
	q.messages[sessionID] = append(q.messages[sessionID], content)
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
	return append([]string(nil), items...)
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
