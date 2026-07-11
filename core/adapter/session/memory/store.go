package memory

import (
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"

	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	agentplan "myai/core/plan"
	domainsession "myai/core/session"
)

type Session = domainsession.Session
type PermissionMode = domainsession.PermissionMode
type AgentMode = domainsession.AgentMode

const (
	PermissionModeAsk = domainsession.PermissionModeAsk
	AgentModeChat     = domainsession.AgentModeChat
)

var (
	NormalizePermissionMode = domainsession.NormalizePermissionMode
	NormalizeAgentMode      = domainsession.NormalizeAgentMode
)

type Store struct {
	mu               sync.RWMutex
	currentSessionId string
	currentModelId   string
	session          map[string]*Session
}

func NewStore(modelID string) *Store {
	if modelID == "" {
		modelID = "gpt-5.5"
	}

	return &Store{
		currentModelId:   modelID,
		currentSessionId: "",
		session:          make(map[string]*Session),
	}
}

func (sm *Store) NewSession() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.currentSessionId = uuid.NewString()
	sm.session[sm.currentSessionId] = newSession(sm.currentSessionId, sm.currentModelId, AgentModeChat, PermissionModeAsk, 0, "", 0, llm.TokenUsage{}, llm.TokenUsage{}, nil)
	sm.currentModelId = sm.session[sm.currentSessionId].Model
	return nil
}

func (sm *Store) PutSession(sessionID string, modelID string, messages []domainmessage.Message) error {
	return sm.PutSessionWithOptions(sessionID, modelID, PermissionModeAsk, 0, messages)
}

func (sm *Store) PutSessionWithPermission(sessionID string, modelID string, permissionMode PermissionMode, messages []domainmessage.Message) error {
	return sm.PutSessionWithOptions(sessionID, modelID, permissionMode, 0, messages)
}

func (sm *Store) PutSessionWithOptions(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, messages []domainmessage.Message) error {
	return sm.PutSessionWithState(sessionID, modelID, permissionMode, contextWindowK, "", 0, messages)
}

func (sm *Store) PutSessionWithState(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, messages []domainmessage.Message) error {
	return sm.PutSessionWithUsage(sessionID, modelID, permissionMode, contextWindowK, summary, compactedMessages, llm.TokenUsage{}, llm.TokenUsage{}, messages)
}

func (sm *Store) PutSessionWithUsage(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error {
	return sm.PutSessionWithModeUsage(sessionID, modelID, AgentModeChat, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages)
}

func (sm *Store) PutSessionWithUsageNoCurrent(sessionID string, modelID string, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error {
	return sm.PutSessionWithModeUsageNoCurrent(sessionID, modelID, AgentModeChat, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages)
}

func (sm *Store) PutSessionWithModeUsage(sessionID string, modelID string, agentMode AgentMode, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error {
	return sm.putSessionWithModeUsage(sessionID, modelID, agentMode, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages, true)
}

func (sm *Store) PutSessionWithModeUsageNoCurrent(sessionID string, modelID string, agentMode AgentMode, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) error {
	return sm.putSessionWithModeUsage(sessionID, modelID, agentMode, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages, false)
}

func (sm *Store) putSessionWithModeUsage(sessionID string, modelID string, agentMode AgentMode, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message, setCurrent bool) error {
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
	sm.session[sessionID] = newSession(sessionID, modelID, agentMode, permissionMode, contextWindowK, summary, compactedMessages, usage, lastUsage, messages)
	return nil
}

func (sm *Store) UseSession(sessionID string) error {
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

func (sm *Store) AddUserMessage(input string) error {
	return sm.AddUserMessageTo(sm.CurrentSessionId(), input)
}

func (sm *Store) AddUserMessageTo(sessionID string, input string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.AddUserMessage(input)
	return nil
}

func (sm *Store) AddAssistantMessage(content string) error {
	return sm.AddAssistantMessageTo(sm.CurrentSessionId(), content)
}

func (sm *Store) AddAssistantMessageTo(sessionID string, content string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.AddAssistantMessage(content)
	return nil
}

func (sm *Store) TrimAfterLastUserMessage(sessionID string) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return "", err
	}

	lastUserIndex := -1
	for index := len(session.Messages) - 1; index >= 0; index-- {
		if session.Messages[index].Role == domainmessage.RoleUser {
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

	session.Messages = append([]domainmessage.Message(nil), session.Messages[:lastUserIndex+1]...)
	session.LastUsage = llm.TokenUsage{}
	return input, nil
}

func (sm *Store) AddUsage(usage llm.TokenUsage) error {
	return sm.AddUsageTo(sm.CurrentSessionId(), usage)
}

func (sm *Store) AddUsageTo(sessionID string, usage llm.TokenUsage) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.AddUsage(usage)
	return nil
}

func (sm *Store) Messages() ([]domainmessage.Message, error) {
	session, err := sm.Current()
	if err != nil {
		return nil, err
	}

	return session.Messages, nil
}

func (sm *Store) ClearCurrent() error {
	return sm.ClearSession(sm.CurrentSessionId())
}

func (sm *Store) ClearSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.Clear()
	return nil
}

func (sm *Store) RemoveSession(sessionID string) error {
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

func (sm *Store) SwitchModel(modelID string) error {
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

func (sm *Store) SwitchModelForSession(sessionID string, modelID string) error {
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

func (sm *Store) SetPermissionMode(mode PermissionMode) error {
	return sm.SetPermissionModeForSession(sm.CurrentSessionId(), mode)
}

func (sm *Store) SetPermissionModeForSession(sessionID string, mode PermissionMode) error {
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

func (sm *Store) SetAgentMode(mode AgentMode) error {
	return sm.SetAgentModeForSession(sm.CurrentSessionId(), mode)
}

func (sm *Store) SetAgentModeForSession(sessionID string, mode AgentMode) error {
	mode = NormalizeAgentMode(mode)
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.AgentMode = mode
	return nil
}

func (sm *Store) SetCurrentPlanForSession(sessionID string, currentPlan *agentplan.Plan) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.CurrentPlan = agentplan.Clone(currentPlan)
	return nil
}

func (sm *Store) SetContextWindowK(windowK int) error {
	return sm.SetContextWindowKForSession(sm.CurrentSessionId(), windowK)
}

func (sm *Store) SetContextWindowKForSession(sessionID string, windowK int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.sessionByIDLocked(sessionID)
	if err != nil {
		return err
	}

	session.ContextWindowK = windowK
	return nil
}

func (sm *Store) SetSummary(summary string, compactedMessages int) error {
	return sm.SetSummaryForSession(sm.CurrentSessionId(), summary, compactedMessages)
}

func (sm *Store) SetSummaryForSession(sessionID string, summary string, compactedMessages int) error {
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

func (sm *Store) Current() (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.getSessionLocked(sm.currentSessionId)
}

func (sm *Store) GetSession(sessionId string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.getSessionLocked(sessionId)
}

func (sm *Store) getSessionLocked(sessionId string) (*Session, error) {
	session := sm.session[sessionId]
	if session == nil {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (sm *Store) CurrentSessionId() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.currentSessionId
}

func (sm *Store) CurrentModelId() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.currentSessionId != "" {
		if session := sm.session[sm.currentSessionId]; session != nil && session.Model != "" {
			return session.Model
		}
	}
	return sm.currentModelId
}

func (sm *Store) CurrentPermissionMode() PermissionMode {
	session, err := sm.Current()
	if err != nil {
		return PermissionModeAsk
	}
	return NormalizePermissionMode(session.PermissionMode)
}

func (sm *Store) CurrentAgentMode() AgentMode {
	session, err := sm.Current()
	if err != nil {
		return AgentModeChat
	}
	return NormalizeAgentMode(session.AgentMode)
}

func (sm *Store) CurrentPlan() *agentplan.Plan {
	session, err := sm.Current()
	if err != nil {
		return nil
	}
	return agentplan.Clone(session.CurrentPlan)
}

func (sm *Store) CurrentContextWindowK() int {
	session, err := sm.Current()
	if err != nil {
		return 0
	}
	return session.ContextWindowK
}

func (sm *Store) CurrentUsage() llm.TokenUsage {
	session, err := sm.Current()
	if err != nil {
		return llm.TokenUsage{}
	}
	return session.Usage
}

func (sm *Store) CurrentLastUsage() llm.TokenUsage {
	session, err := sm.Current()
	if err != nil {
		return llm.TokenUsage{}
	}
	return session.LastUsage
}

func (sm *Store) sessionByIDLocked(sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}

	session := sm.session[sessionID]
	if session == nil {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func textFromMessage(message domainmessage.Message) string {
	return strings.Join([]string{message.Text()}, "\n")
}

func newSession(id, model string, agentMode AgentMode, permissionMode PermissionMode, contextWindowK int, summary string, compactedMessages int, usage llm.TokenUsage, lastUsage llm.TokenUsage, messages []domainmessage.Message) *Session {
	return domainsession.NewFromState(domainsession.InitialState{
		ID:                id,
		Model:             model,
		AgentMode:         agentMode,
		PermissionMode:    permissionMode,
		ContextWindowK:    contextWindowK,
		Summary:           summary,
		CompactedMessages: compactedMessages,
		Usage:             usage,
		LastUsage:         lastUsage,
		Messages:          messages,
	})
}
