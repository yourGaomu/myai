package session

import (
	"errors"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"
)

type SessionManage struct {
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
	sm.currentSessionId = uuid.NewString()
	sm.session[sm.currentSessionId] = newSession(sm.currentSessionId, sm.currentModelId, PermissionModeAsk, 0, "", 0, nil)
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
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if modelID == "" {
		modelID = sm.currentModelId
	}

	sm.currentSessionId = sessionID
	sm.currentModelId = modelID
	sm.session[sessionID] = newSession(sessionID, modelID, permissionMode, contextWindowK, summary, compactedMessages, messages)
	return nil
}

func (sm *SessionManage) UseSession(sessionID string) error {
	if _, err := sm.GetSession(sessionID); err != nil {
		return err
	}

	sm.currentSessionId = sessionID
	return nil
}

func (sm *SessionManage) AddUserMessage(input string) error {
	session, err := sm.Current()
	if err != nil {
		return err
	}

	session.AddUserMessage(input)
	return nil
}

func (sm *SessionManage) AddAssistantMessage(content string) error {
	session, err := sm.Current()
	if err != nil {
		return err
	}

	session.AddAssistantMessage(content)
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
	session, err := sm.Current()
	if err != nil {
		return err
	}

	session.Clear()
	return nil
}

func (sm *SessionManage) SwitchModel(modelID string) error {
	if modelID == "" {
		return errors.New("model id is empty")
	}

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

func (sm *SessionManage) SetPermissionMode(mode PermissionMode) error {
	mode = NormalizePermissionMode(mode)
	session, err := sm.Current()
	if err != nil {
		return err
	}

	session.PermissionMode = mode
	return nil
}

func (sm *SessionManage) SetContextWindowK(windowK int) error {
	session, err := sm.Current()
	if err != nil {
		return err
	}

	session.ContextWindowK = windowK
	return nil
}

func (sm *SessionManage) SetSummary(summary string, compactedMessages int) error {
	session, err := sm.Current()
	if err != nil {
		return err
	}

	session.Summary = summary
	session.CompactedMessages = compactedMessages
	return nil
}

func (sm *SessionManage) Current() (*Session, error) {
	return sm.GetSession(sm.currentSessionId)
}

func (sm *SessionManage) GetSession(sessionId string) (*Session, error) {
	session := sm.session[sessionId]
	if session == nil {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (sm *SessionManage) CurrentSessionId() string {
	return sm.currentSessionId
}

func (sm *SessionManage) CurrentModelId() string {
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
