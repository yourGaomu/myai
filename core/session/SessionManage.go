package session

import (
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"

	"myai/core/llm"
)

type SessionManage struct {
	mu               sync.RWMutex
	currentSessionId string
	currentModelId   string
	session          map[string]*Session
}

func NewSessionManage(modelID string) *SessionManage {
	if modelID == "" {
		modelID = "gpt-5.5"
	}

	return &SessionManage{
		currentModelId:   modelID,
		currentSessionId: "",
		session:          make(map[string]*Session),
	}
}

func (sm *SessionManage) NewSession() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.currentSessionId = uuid.NewString()
	sm.session[sm.currentSessionId] = newSession(sm.currentSessionId, sm.currentModelId, PermissionModeAsk, 0, "", 0, llm.TokenUsage{}, llm.TokenUsage{}, nil)
	sm.currentModelId = sm.session[sm.currentSessionId].Model
	return nil
}

func (sm *SessionManage) PutSession(sessionID string, modelID string, messages []llms.MessageContent) error {
	return sm.PutSessionWithOptions(sessionID, modelID, PermissionModeAsk, 0, messages)
}

func (sm *SessionManage) PutSessionWithPermission(sessionID string, modelID string, permissionMode PermissionMode, messages []llms.MessageContent) error {
	return sm.PutSessionWithOptions(sessionID, modelID, permissionMode, 0, messages)
}

func (sm *SessionManage) PutSessionWithOptions(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, messages []llms.MessageContent) error {
	return sm.PutSessionWithState(sessionID, modelID, permissionMode, contextWindowK, "", 0, messages)
}

func (sm *SessionManage) PutSessionWithState(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, messages []llms.MessageContent) error {
	return sm.PutSessionWithUsage(sessionID, modelID, permissionMode, contextWindowK, summary, compactedMessages, llm.TokenUsage{}, llm.TokenUsage{}, messages)
}

func (sm *SessionManage) PutSessionWithUsage(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []llms.MessageContent) error {
	return sm.putSessionWithUsage(sessionID, modelID, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages, true)
}

func (sm *SessionManage) PutSessionWithUsageNoCurrent(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []llms.MessageContent) error {
	return sm.putSessionWithUsage(sessionID, modelID, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages, false)
}

func (sm *SessionManage) putSessionWithUsage(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []llms.MessageContent, setCurrent bool) error {
	if sessionID == "" {
		return errors.New("session id is empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if modelID == "" {
		modelID = sm.currentModelId
	}

	if setCurrent {
		sm.currentSessionId = sessionID
		sm.currentModelId = modelID
	}
	sm.session[sessionID] = newSession(sessionID, modelID, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages)
	return nil
}

func (sm *SessionManage) UseSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session := sm.session[sessionID]
	if session == nil {
		return errors.New("session not found")
	}

	sm.currentSessionId = sessionID
	if session.Model != "" {
		sm.currentModelId = session.Model
	}
	return nil
}

func (sm *SessionManage) AddUserMessage(input string) error {
	return sm.AddUserMessageTo(sm.CurrentSessionId(), input)
}

func (sm *SessionManage) AddUserMessageTo(sessionID string, input string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.AddUserMessage(input)
	return nil
}

func (sm *SessionManage) AddAssistantMessage(content string) error {
	return sm.AddAssistantMessageTo(sm.CurrentSessionId(), content)
}

func (sm *SessionManage) AddAssistantMessageTo(sessionID string, content string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.AddAssistantMessage(content)
	return nil
}

func (sm *SessionManage) TrimAfterLastUserMessage(sessionID string) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return "", err
	}

	lastUserIndex := -1
	for index := len(session.Messages) - 1; index >= 0; index-- {
		if session.Messages[index].Role == llms.ChatMessageTypeHuman {
			lastUserIndex = index
			break
		}
	}
	if lastUserIndex < 0 {
		return "", errors.New("no user message to regenerate")
	}

	input := textFromMessage(session.Messages[lastUserIndex])
	if strings.TrimSpace(input) == "" {
		return "", errors.New("last user message is empty")
	}

	session.Messages = append([]llms.MessageContent(nil), session.Messages[:lastUserIndex+1]...)
	session.LastUsage = llm.TokenUsage{}
	return input, nil
}

func (sm *SessionManage) AddUsage(usage llm.TokenUsage) error {
	return sm.AddUsageTo(sm.CurrentSessionId(), usage)
}

func (sm *SessionManage) AddUsageTo(sessionID string, usage llm.TokenUsage) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.AddUsage(usage)
	return nil
}

func (sm *SessionManage) Messages() ([]llms.MessageContent, error) {
	session, err := sm.Current()
	if err != nil {
		return nil, err
	}

	return session.Messages, nil
}

func (sm *SessionManage) ClearCurrent() error {
	return sm.ClearSession(sm.CurrentSessionId())
}

func (sm *SessionManage) ClearSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.Clear()
	return nil
}

func (sm *SessionManage) RemoveSession(sessionID string) error {
	if sessionID == "" {
		return errors.New("session id is empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.session, sessionID)
	if sm.currentSessionId == sessionID {
		sm.currentSessionId = ""
	}
	return nil
}

func (sm *SessionManage) SwitchModel(modelID string) error {
	if modelID == "" {
		return errors.New("model id is empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.currentModelId = modelID
	if sm.currentSessionId == "" {
		return nil
	}

	session := sm.session[sm.currentSessionId]
	if session != nil {
		session.Model = modelID
	}
	return nil
}

func (sm *SessionManage) SwitchModelForSession(sessionID string, modelID string) error {
	if modelID == "" {
		return errors.New("model id is empty")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.Model = modelID
	if sm.currentSessionId == sessionID {
		sm.currentModelId = modelID
	}
	return nil
}

func (sm *SessionManage) SetPermissionMode(mode PermissionMode) error {
	return sm.SetPermissionModeForSession(sm.CurrentSessionId(), mode)
}

func (sm *SessionManage) SetPermissionModeForSession(sessionID string, mode PermissionMode) error {
	mode = NormalizePermissionMode(mode)
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.PermissionMode = mode
	return nil
}

func (sm *SessionManage) SetContextWindowK(windowK int) error {
	return sm.SetContextWindowKForSession(sm.CurrentSessionId(), windowK)
}

func (sm *SessionManage) SetContextWindowKForSession(sessionID string, windowK int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.ContextWindowK = windowK
	return nil
}

func (sm *SessionManage) SetSummary(summary string, compactedMessages int) error {
	return sm.SetSummaryForSession(sm.CurrentSessionId(), summary, compactedMessages)
}

func (sm *SessionManage) SetSummaryForSession(sessionID string, summary string, compactedMessages int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.Summary = summary
	session.CompactedMessages = compactedMessages
	return nil
}

func (sm *SessionManage) Current() (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.getSessionLocked(sm.currentSessionId)
}

func (sm *SessionManage) GetSession(sessionId string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.getSessionLocked(sessionId)
}

func (sm *SessionManage) getSessionLocked(sessionId string) (*Session, error) {
	session := sm.session[sessionId]
	if session == nil {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (sm *SessionManage) CurrentSessionId() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.currentSessionId
}

func (sm *SessionManage) CurrentModelId() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.currentSessionId != "" {
		if session := sm.session[sm.currentSessionId]; session != nil && session.Model != "" {
			return session.Model
		}
	}
	return sm.currentModelId
}

func (sm *SessionManage) CurrentPermissionMode() PermissionMode {
	session, err := sm.Current()
	if err != nil {
		return PermissionModeAsk
	}
	return NormalizePermissionMode(session.PermissionMode)
}

func (sm *SessionManage) CurrentContextWindowK() int {
	session, err := sm.Current()
	if err != nil {
		return 0
	}
	return session.ContextWindowK
}

func (sm *SessionManage) CurrentUsage() llm.TokenUsage {
	session, err := sm.Current()
	if err != nil {
		return llm.TokenUsage{}
	}
	return session.Usage
}

func (sm *SessionManage) CurrentLastUsage() llm.TokenUsage {
	session, err := sm.Current()
	if err != nil {
		return llm.TokenUsage{}
	}
	return session.LastUsage
}

func (sm *SessionManage) sessionByIDLocked(sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}

	session := sm.session[sessionID]
	if session == nil {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func textFromMessage(message llms.MessageContent) string {
	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		if text, ok := part.(llms.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
