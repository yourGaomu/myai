package protocol

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	TypeAgentOnline                MessageType = "agent_online"
	TypeAgentOffline               MessageType = "agent_offline"
	TypeUserMessage                MessageType = "user_message"
	TypeAssistantDelta             MessageType = "assistant_delta"
	TypeAssistantDone              MessageType = "assistant_done"
	TypeToolCall                   MessageType = "tool_call"
	TypePermissionAsk              MessageType = "permission_ask"
	TypePermissionResult           MessageType = "permission_result"
	TypeSessionList                MessageType = "session_list"
	TypeSessionListResult          MessageType = "session_list_result"
	TypeSessionNew                 MessageType = "session_new"
	TypeSessionLoad                MessageType = "session_load"
	TypeSessionDelete              MessageType = "session_delete"
	TypeSessionDeleteResult        MessageType = "session_delete_result"
	TypeSessionRestore             MessageType = "session_restore"
	TypeSessionRestoreResult       MessageType = "session_restore_result"
	TypeSessionChanged             MessageType = "session_changed"
	TypeSessionHistory             MessageType = "session_history"
	TypeSessionHistoryResult       MessageType = "session_history_result"
	TypeSessionPermissionSet       MessageType = "session_permission_set"
	TypeSessionPermissionSetResult MessageType = "session_permission_set_result"
	TypeSessionContextSet          MessageType = "session_context_set"
	TypeSessionContextSetResult    MessageType = "session_context_set_result"
	TypeSessionCompact             MessageType = "session_compact"
	TypeSessionCompactResult       MessageType = "session_compact_result"
	TypeSessionPause               MessageType = "session_pause"
	TypeSessionPauseResult         MessageType = "session_pause_result"
	TypeModelList                  MessageType = "model_list"
	TypeModelListResult            MessageType = "model_list_result"
	TypeModelSwitch                MessageType = "model_switch"
	TypeModelSwitchResult          MessageType = "model_switch_result"
	TypeSkillList                  MessageType = "skill_list"
	TypeSkillListResult            MessageType = "skill_list_result"
	TypeSkillReload                MessageType = "skill_reload"
	TypeSkillReloadResult          MessageType = "skill_reload_result"
	TypeFileList                   MessageType = "file_list"
	TypeFileListResult             MessageType = "file_list_result"
	TypeFileRead                   MessageType = "file_read"
	TypeFileReadResult             MessageType = "file_read_result"
	TypeChangesList                MessageType = "changes_list"
	TypeChangesListResult          MessageType = "changes_list_result"
	TypeChangeDiff                 MessageType = "change_diff"
	TypeChangeDiffResult           MessageType = "change_diff_result"
	TypeChangeRevert               MessageType = "change_revert"
	TypeChangeRevertResult         MessageType = "change_revert_result"
	TypeHistoryList                MessageType = "history_list"
	TypeHistoryListResult          MessageType = "history_list_result"
	TypeHistoryDiff                MessageType = "history_diff"
	TypeHistoryDiffResult          MessageType = "history_diff_result"
	TypeHistoryRevert              MessageType = "history_revert"
	TypeHistoryRevertResult        MessageType = "history_revert_result"
	TypeError                      MessageType = "error"
	TypeHeartbeat                  MessageType = "heartbeat"
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
	Content string      `json:"content"`
	Usage   TokenUsage  `json:"usage,omitempty"`
	Context ContextInfo `json:"context,omitempty"`
	Compact CompactInfo `json:"compact,omitempty"`
	Paused  bool        `json:"paused,omitempty"`
	Message string      `json:"message,omitempty"`
}

type TokenUsage struct {
	PromptTokens       int  `json:"prompt_tokens,omitempty"`
	CompletionTokens   int  `json:"completion_tokens,omitempty"`
	TotalTokens        int  `json:"total_tokens,omitempty"`
	ReasoningTokens    int  `json:"reasoning_tokens,omitempty"`
	PromptCachedTokens int  `json:"prompt_cached_tokens,omitempty"`
	Available          bool `json:"available,omitempty"`
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

type SessionListPayload struct {
	IncludeDeleted bool `json:"include_deleted,omitempty"`
}

type SessionLoadPayload struct {
	SessionID string `json:"session_id"`
}

type SessionDeletePayload struct {
	SessionID string `json:"session_id"`
}

type SessionRestorePayload struct {
	SessionID string `json:"session_id"`
}

type SessionSummary struct {
	ID             string      `json:"id"`
	Title          string      `json:"title"`
	Model          string      `json:"model"`
	PermissionMode string      `json:"permission_mode"`
	ContextWindowK int         `json:"context_window_k"`
	Usage          *TokenUsage `json:"usage,omitempty"`
	LastUsage      *TokenUsage `json:"last_usage,omitempty"`
	Deleted        bool        `json:"deleted,omitempty"`
	DeletedAt      *time.Time  `json:"deleted_at,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type SessionListResultPayload struct {
	CurrentSessionID string           `json:"current_session_id"`
	Sessions         []SessionSummary `json:"sessions"`
	IncludeDeleted   bool             `json:"include_deleted,omitempty"`
}

type SessionChangedPayload struct {
	CurrentSessionID string           `json:"current_session_id"`
	Session          SessionSummary   `json:"session"`
	Sessions         []SessionSummary `json:"sessions"`
}

type SessionHistoryPayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type SessionHistoryMessage struct {
	ID            string     `json:"id"`
	Role          string     `json:"role"`
	Content       string     `json:"content,omitempty"`
	Reasoning     string     `json:"reasoning,omitempty"`
	ToolCallID    string     `json:"tool_call_id,omitempty"`
	ToolName      string     `json:"tool_name,omitempty"`
	ToolArguments string     `json:"tool_arguments,omitempty"`
	ToolError     string     `json:"tool_error,omitempty"`
	Usage         TokenUsage `json:"usage,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type SessionHistoryResultPayload struct {
	SessionID string                  `json:"session_id"`
	Messages  []SessionHistoryMessage `json:"messages"`
	Count     int                     `json:"count"`
}

type SessionPermissionSetPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Mode      string `json:"mode"`
}

type SessionContextSetPayload struct {
	SessionID string `json:"session_id,omitempty"`
	WindowK   int    `json:"window_k"`
}

type SessionCompactPayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type SessionPausePayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type SessionPauseResultPayload struct {
	SessionID string `json:"session_id"`
	Paused    bool   `json:"paused"`
	Message   string `json:"message,omitempty"`
}

type ContextInfo struct {
	WindowK           int    `json:"window_k"`
	FullTokens        int    `json:"full_tokens"`
	SelectedTokens    int    `json:"selected_tokens"`
	SummaryTokens     int    `json:"summary_tokens"`
	PrefixTokens      int    `json:"prefix_tokens"`
	CacheableTokens   int    `json:"cacheable_tokens"`
	FullMessages      int    `json:"full_messages"`
	SelectedMessages  int    `json:"selected_messages"`
	CompactedMessages int    `json:"compacted_messages"`
	HasSummary        bool   `json:"has_summary"`
	Truncated         bool   `json:"truncated"`
	SummaryVersion    int    `json:"summary_version"`
	SummaryHash       string `json:"summary_hash,omitempty"`
	PrefixHash        string `json:"prefix_hash,omitempty"`
}

type CompactInfo struct {
	Triggered         bool   `json:"triggered,omitempty"`
	Reason            string `json:"reason,omitempty"`
	BeforeTokens      int    `json:"before_tokens,omitempty"`
	AfterTokens       int    `json:"after_tokens,omitempty"`
	NewMessages       int    `json:"new_messages,omitempty"`
	CompactedMessages int    `json:"compacted_messages,omitempty"`
	SummaryTokens     int    `json:"summary_tokens,omitempty"`
	SummaryVersion    int    `json:"summary_version,omitempty"`
	SummaryHash       string `json:"summary_hash,omitempty"`
	PrefixHash        string `json:"prefix_hash,omitempty"`
	CacheableTokens   int    `json:"cacheable_tokens,omitempty"`
}

type SessionSettingsResultPayload struct {
	CurrentSessionID string           `json:"current_session_id"`
	Session          SessionSummary   `json:"session"`
	Sessions         []SessionSummary `json:"sessions"`
	Context          ContextInfo      `json:"context,omitempty"`
	Message          string           `json:"message,omitempty"`
}

type ModelSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Provider  string `json:"provider,omitempty"`
	ModelName string `json:"model_name,omitempty"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"is_default"`
}

type ModelListPayload struct{}

type ModelListResultPayload struct {
	CurrentModelID string         `json:"current_model_id"`
	Models         []ModelSummary `json:"models"`
}

type ModelSwitchPayload struct {
	ModelID string `json:"model_id"`
}

type ModelSwitchResultPayload struct {
	CurrentModelID string         `json:"current_model_id"`
	Models         []ModelSummary `json:"models"`
	Session        SessionSummary `json:"session"`
	Message        string         `json:"message,omitempty"`
}

type SkillListPayload struct{}

type SkillReloadPayload struct{}

type SkillSummary struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Path        string    `json:"path"`
	Triggers    []string  `json:"triggers,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SkillListResultPayload struct {
	Root     string         `json:"root,omitempty"`
	Skills   []SkillSummary `json:"skills"`
	Count    int            `json:"count"`
	Reloaded bool           `json:"reloaded,omitempty"`
	Message  string         `json:"message,omitempty"`
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
	Restorable     bool   `json:"restorable"`
}

type ChangesListResultPayload struct {
	Repository bool          `json:"repository"`
	Source     string        `json:"source,omitempty"`
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
	Path       string `json:"path"`
	Diff       string `json:"diff,omitempty"`
	Truncated  bool   `json:"truncated"`
	Binary     bool   `json:"binary"`
	Restorable bool   `json:"restorable"`
	Message    string `json:"message,omitempty"`
}

type ChangeRevertPayload struct {
	Path string `json:"path"`
}

type ChangeRevertResultPayload struct {
	Path     string `json:"path"`
	Reverted bool   `json:"reverted"`
	Message  string `json:"message,omitempty"`
}

type HistoryListPayload struct {
	Limit int `json:"limit,omitempty"`
}

type HistoryCheckpoint struct {
	ID          string    `json:"id"`
	Title       string    `json:"title,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	RequestID   string    `json:"request_id,omitempty"`
	ChangeCount int       `json:"change_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type HistoryListResultPayload struct {
	Root        string              `json:"root,omitempty"`
	Checkpoints []HistoryCheckpoint `json:"checkpoints"`
	Count       int                 `json:"count"`
}

type HistoryDiffPayload struct {
	CheckpointID string `json:"checkpoint_id"`
}

type HistoryFileDiff struct {
	Path       string `json:"path"`
	ChangeType string `json:"change_type"`
	Diff       string `json:"diff,omitempty"`
	Truncated  bool   `json:"truncated"`
	Binary     bool   `json:"binary"`
	Restorable bool   `json:"restorable"`
	Message    string `json:"message,omitempty"`
}

type HistoryDiffResultPayload struct {
	CheckpointID string            `json:"checkpoint_id"`
	Files        []HistoryFileDiff `json:"files"`
	Count        int               `json:"count"`
	Message      string            `json:"message,omitempty"`
}

type HistoryRevertPayload struct {
	CheckpointID string `json:"checkpoint_id"`
}

type HistoryRevertResultPayload struct {
	CheckpointID string   `json:"checkpoint_id"`
	Reverted     bool     `json:"reverted"`
	Paths        []string `json:"paths"`
	Message      string   `json:"message,omitempty"`
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
