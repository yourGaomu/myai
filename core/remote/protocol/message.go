package protocol

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	TypeAgentOnline       MessageType = "agent_online"
	TypeAgentOffline      MessageType = "agent_offline"
	TypeUserMessage       MessageType = "user_message"
	TypeAssistantDelta    MessageType = "assistant_delta"
	TypeAssistantDone     MessageType = "assistant_done"
	TypeToolCall          MessageType = "tool_call"
	TypePermissionAsk     MessageType = "permission_ask"
	TypePermissionResult  MessageType = "permission_result"
	TypeSessionList       MessageType = "session_list"
	TypeSessionListResult MessageType = "session_list_result"
	TypeSessionNew        MessageType = "session_new"
	TypeSessionLoad       MessageType = "session_load"
	TypeSessionChanged    MessageType = "session_changed"
	TypeFileList          MessageType = "file_list"
	TypeFileListResult    MessageType = "file_list_result"
	TypeFileRead          MessageType = "file_read"
	TypeFileReadResult    MessageType = "file_read_result"
	TypeChangesList       MessageType = "changes_list"
	TypeChangesListResult MessageType = "changes_list_result"
	TypeChangeDiff        MessageType = "change_diff"
	TypeChangeDiffResult  MessageType = "change_diff_result"
	TypeError             MessageType = "error"
	TypeHeartbeat         MessageType = "heartbeat"
)

type Message struct {
	Type        MessageType     `json:"type"`
	RequestID   string          `json:"request_id,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	DeviceID    string          `json:"device_id,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	ClientToken string          `json:"client_token,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

type AgentOnlinePayload struct {
	Status   string `json:"status"`
	BindCode string `json:"bind_code,omitempty"`
}

type UserMessagePayload struct {
	Content string `json:"content"`
}

type AssistantDeltaPayload struct {
	Content string `json:"content"`
}

type AssistantDonePayload struct {
	Content string `json:"content"`
}

type ToolCallPayload struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type PermissionAskPayload struct {
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
	Permission string `json:"permission"`
}

type PermissionResultPayload struct {
	Allowed bool `json:"allowed"`
}

type SessionLoadPayload struct {
	SessionID string `json:"session_id"`
}

type SessionSummary struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Model          string    `json:"model"`
	PermissionMode string    `json:"permission_mode"`
	ContextWindowK int       `json:"context_window_k"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SessionListResultPayload struct {
	CurrentSessionID string           `json:"current_session_id"`
	Sessions         []SessionSummary `json:"sessions"`
}

type SessionChangedPayload struct {
	CurrentSessionID string           `json:"current_session_id"`
	Session          SessionSummary   `json:"session"`
	Sessions         []SessionSummary `json:"sessions"`
}

type FileListPayload struct {
	Path          string `json:"path"`
	IncludeHidden bool   `json:"include_hidden,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type FileEntry struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Size     int64     `json:"size,omitempty"`
	Modified time.Time `json:"modified_at,omitempty"`
}

type FileListResultPayload struct {
	Path      string      `json:"path"`
	Parent    string      `json:"parent,omitempty"`
	Entries   []FileEntry `json:"entries"`
	Count     int         `json:"count"`
	Truncated bool        `json:"truncated"`
}

type FileReadPayload struct {
	Path string `json:"path"`
}

type FileReadResultPayload struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Language  string `json:"language"`
	Content   string `json:"content,omitempty"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}

type ChangesListPayload struct {
	Limit int `json:"limit,omitempty"`
}

type ChangeEntry struct {
	Path           string `json:"path"`
	OldPath        string `json:"old_path,omitempty"`
	Status         string `json:"status"`
	IndexStatus    string `json:"index_status,omitempty"`
	WorktreeStatus string `json:"worktree_status,omitempty"`
	Staged         bool   `json:"staged"`
	Unstaged       bool   `json:"unstaged"`
	Untracked      bool   `json:"untracked"`
	Deleted        bool   `json:"deleted"`
	Renamed        bool   `json:"renamed"`
}

type ChangesListResultPayload struct {
	Repository bool          `json:"repository"`
	Root       string        `json:"root,omitempty"`
	Entries    []ChangeEntry `json:"entries"`
	Count      int           `json:"count"`
	Truncated  bool          `json:"truncated"`
	Clean      bool          `json:"clean"`
	Message    string        `json:"message,omitempty"`
}

type ChangeDiffPayload struct {
	Path string `json:"path"`
}

type ChangeDiffResultPayload struct {
	Path      string `json:"path"`
	Diff      string `json:"diff,omitempty"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
	Message   string `json:"message,omitempty"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

func NewMessage(
	messageType MessageType,
	requestID string,
	userID string,
	deviceID string,
	sessionID string,
	payload any,
) (Message, error) {
	var raw json.RawMessage

	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return Message{}, err
		}
		raw = data
	}

	return Message{
		Type:      messageType,
		RequestID: requestID,
		UserID:    userID,
		DeviceID:  deviceID,
		SessionID: sessionID,
		Payload:   raw,
	}, nil
}

func DecodePayload[T any](message Message) (T, error) {
	var payload T
	if len(message.Payload) == 0 {
		return payload, nil
	}

	err := json.Unmarshal(message.Payload, &payload)
	return payload, err
}
