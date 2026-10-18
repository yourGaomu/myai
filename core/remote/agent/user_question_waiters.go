package agent

import (
	"errors"
	"sort"
	"strings"
	"sync"

	domaininteraction "myai/core/domain/interaction"
)

type userQuestionWaiter struct {
	request domaininteraction.Request
	answer  chan domaininteraction.Answer
}

type userQuestionWaiterRegistry struct {
	mu      sync.Mutex
	waiters map[string]*userQuestionWaiter
}

func newUserQuestionWaiterRegistry() *userQuestionWaiterRegistry {
	return &userQuestionWaiterRegistry{waiters: make(map[string]*userQuestionWaiter)}
}

func (registry *userQuestionWaiterRegistry) register(request domaininteraction.Request) (<-chan domaininteraction.Answer, error) {
	if registry == nil {
		return nil, errors.New("user question registry is nil")
	}
	waiter := &userQuestionWaiter{request: request, answer: make(chan domaininteraction.Answer, 1)}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.waiters[request.ID]; exists {
		return nil, errors.New("user question id is already active")
	}
	registry.waiters[request.ID] = waiter
	return waiter.answer, nil
}

func (registry *userQuestionWaiterRegistry) resolve(questionID string, answer domaininteraction.Answer) bool {
	if registry == nil {
		return false
	}
	registry.mu.Lock()
	waiter := registry.waiters[questionID]
	if waiter == nil || domaininteraction.ValidateAnswer(waiter.request, answer) != nil {
		registry.mu.Unlock()
		return false
	}
	delete(registry.waiters, questionID)
	registry.mu.Unlock()
	waiter.answer <- answer
	return true
}

func (registry *userQuestionWaiterRegistry) list(sessionID string, knownQuestionIDs []string) ([]domaininteraction.Request, []string) {
	if registry == nil {
		return nil, nil
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	requests := make([]domaininteraction.Request, 0, len(registry.waiters))
	activeIDs := make(map[string]struct{}, len(registry.waiters))
	for _, waiter := range registry.waiters {
		activeIDs[waiter.request.ID] = struct{}{}
		if sessionID != "" && waiter.request.SessionID != sessionID {
			continue
		}
		request := waiter.request
		request.Questions = append([]domaininteraction.Question(nil), request.Questions...)
		for index := range request.Questions {
			request.Questions[index].Options = append([]string(nil), request.Questions[index].Options...)
		}
		requests = append(requests, request)
	}
	resolvedIDs := make([]string, 0)
	seenIDs := make(map[string]struct{}, len(knownQuestionIDs))
	for _, id := range knownQuestionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, seen := seenIDs[id]; seen {
			continue
		}
		seenIDs[id] = struct{}{}
		if _, active := activeIDs[id]; !active {
			resolvedIDs = append(resolvedIDs, id)
		}
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].ID < requests[j].ID })
	sort.Strings(resolvedIDs)
	return requests, resolvedIDs
}

func (registry *userQuestionWaiterRegistry) unregister(questionID string, answer <-chan domaininteraction.Answer) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	waiter := registry.waiters[questionID]
	if waiter == nil || (<-chan domaininteraction.Answer)(waiter.answer) != answer {
		return
	}
	delete(registry.waiters, questionID)
}
