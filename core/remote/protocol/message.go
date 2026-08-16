package protocol

import (
	"encoding/json"
	"time"
)

const (
	HeaderAgentUserID   = "X-MyAI-Agent-User-ID"
	HeaderAgentDeviceID = "X-MyAI-Agent-Device-ID"
)

type MessageType string

const (
	TypeAgentOnline                      MessageType = "agent_online"
	TypeAgentOffline                     MessageType = "agent_offline"
	TypeUserMessage                      MessageType = "user_message"
	TypeAssistantDelta                   MessageType = "assistant_delta"
	TypeAssistantDone                    MessageType = "assistant_done"
	TypeAgentRunStarted                  MessageType = "agent_run_started"
	TypeAgentRunEvent                    MessageType = "agent_run_event"
	TypeAgentRunCompleted                MessageType = "agent_run_completed"
	TypeAgentRunList                     MessageType = "agent_run_list"
	TypeAgentRunListResult               MessageType = "agent_run_list_result"
	TypeToolCall                         MessageType = "tool_call"
	TypeToolResult                       MessageType = "tool_result"
	TypePermissionAsk                    MessageType = "permission_ask"
	TypePermissionResult                 MessageType = "permission_result"
	TypeSessionList                      MessageType = "session_list"
	TypeSessionListResult                MessageType = "session_list_result"
	TypeSessionNew                       MessageType = "session_new"
	TypeSessionLoad                      MessageType = "session_load"
	TypeSessionDelete                    MessageType = "session_delete"
	TypeSessionDeleteResult              MessageType = "session_delete_result"
	TypeSessionRestore                   MessageType = "session_restore"
	TypeSessionRestoreResult             MessageType = "session_restore_result"
	TypeSessionChanged                   MessageType = "session_changed"
	TypeSessionHistory                   MessageType = "session_history"
	TypeSessionHistoryResult             MessageType = "session_history_result"
	TypeSessionHistoryMeta               MessageType = "session_history_meta"
	TypeSessionHistoryMetaResult         MessageType = "session_history_meta_result"
	TypeSessionHistoryDelta              MessageType = "session_history_delta"
	TypeSessionHistoryDeltaResult        MessageType = "session_history_delta_result"
	TypeSessionPermissionSet             MessageType = "session_permission_set"
	TypeSessionPermissionSetResult       MessageType = "session_permission_set_result"
	TypeSessionModeSet                   MessageType = "session_mode_set"
	TypeSessionModeSetResult             MessageType = "session_mode_set_result"
	TypeSessionPlanExecute               MessageType = "session_plan_execute"
	TypeSessionPlanExecuteUpdate         MessageType = "session_plan_update"
	TypeSessionPlanExecuteResult         MessageType = "session_plan_execute_result"
	TypeSessionContextQuery              MessageType = "session_context_query"
	TypeSessionContextQueryResult        MessageType = "session_context_query_result"
	TypeSessionContextSet                MessageType = "session_context_set"
	TypeSessionContextSetResult          MessageType = "session_context_set_result"
	TypeSessionRAGSet                    MessageType = "session_rag_set"
	TypeSessionRAGSetResult              MessageType = "session_rag_set_result"
	TypeSessionGenerationQuery           MessageType = "session_generation_query"
	TypeSessionGenerationQueryResult     MessageType = "session_generation_query_result"
	TypeSessionGenerationSet             MessageType = "session_generation_set"
	TypeSessionGenerationSetResult       MessageType = "session_generation_set_result"
	TypeSessionStyleSet                  MessageType = "session_style_set"
	TypeSessionStyleSetResult            MessageType = "session_style_set_result"
	TypeSessionCompact                   MessageType = "session_compact"
	TypeSessionCompactResult             MessageType = "session_compact_result"
	TypeSessionPause                     MessageType = "session_pause"
	TypeSessionPauseResult               MessageType = "session_pause_result"
	TypeSessionRegenerate                MessageType = "session_regenerate"
	TypeModelList                        MessageType = "model_list"
	TypeModelListResult                  MessageType = "model_list_result"
	TypeModelSwitch                      MessageType = "model_switch"
	TypeModelSwitchResult                MessageType = "model_switch_result"
	TypeSkillList                        MessageType = "skill_list"
	TypeSkillListResult                  MessageType = "skill_list_result"
	TypeSkillReload                      MessageType = "skill_reload"
	TypeSkillReloadResult                MessageType = "skill_reload_result"
	TypeAssetList                        MessageType = "asset_list"
	TypeAssetListResult                  MessageType = "asset_list_result"
	TypeKnowledgeCatalogList             MessageType = "knowledge_catalog_list"
	TypeKnowledgeCatalogListResult       MessageType = "knowledge_catalog_list_result"
	TypeKnowledgeCategoryCreate          MessageType = "knowledge_category_create"
	TypeKnowledgeCategoryMove            MessageType = "knowledge_category_move"
	TypeKnowledgeCategoryDelete          MessageType = "knowledge_category_delete"
	TypeKnowledgeBaseCreate              MessageType = "knowledge_base_create"
	TypeKnowledgeBaseUpdate              MessageType = "knowledge_base_update"
	TypeKnowledgeBaseDelete              MessageType = "knowledge_base_delete"
	TypeKnowledgeCatalogMutationResult   MessageType = "knowledge_catalog_mutation_result"
	TypeKnowledgeDocumentList            MessageType = "knowledge_document_list"
	TypeKnowledgeDocumentListResult      MessageType = "knowledge_document_list_result"
	TypeKnowledgeDocumentIngest          MessageType = "knowledge_document_ingest"
	TypeKnowledgeDocumentRetry           MessageType = "knowledge_document_retry"
	TypeKnowledgeDocumentDelete          MessageType = "knowledge_document_delete"
	TypeKnowledgeDocumentMutationResult  MessageType = "knowledge_document_mutation_result"
	TypeKnowledgeProfileList             MessageType = "knowledge_profile_list"
	TypeKnowledgeProfileListResult       MessageType = "knowledge_profile_list_result"
	TypeKnowledgeSearchPreview           MessageType = "knowledge_search_preview"
	TypeKnowledgeSearchPreviewResult     MessageType = "knowledge_search_preview_result"
	TypeAIMemoryList                     MessageType = "ai_memory_list"
	TypeAIMemoryListResult               MessageType = "ai_memory_list_result"
	TypeAIMemoryCreate                   MessageType = "ai_memory_create"
	TypeAIMemoryUpdate                   MessageType = "ai_memory_update"
	TypeAIMemoryDelete                   MessageType = "ai_memory_delete"
	TypeAIMemoryRestore                  MessageType = "ai_memory_restore"
	TypeAIMemoryMutationResult           MessageType = "ai_memory_mutation_result"
	TypeAIMemoryCandidateList            MessageType = "ai_memory_candidate_list"
	TypeAIMemoryCandidateListResult      MessageType = "ai_memory_candidate_list_result"
	TypeAIMemoryCandidateApprove         MessageType = "ai_memory_candidate_approve"
	TypeAIMemoryCandidateReject          MessageType = "ai_memory_candidate_reject"
	TypeAIMemoryCandidateMutationResult  MessageType = "ai_memory_candidate_mutation_result"
	TypeAIMemoryExtractionJobList        MessageType = "ai_memory_extraction_job_list"
	TypeAIMemoryExtractionJobListResult  MessageType = "ai_memory_extraction_job_list_result"
	TypeAIMemoryExtractionJobRetry       MessageType = "ai_memory_extraction_job_retry"
	TypeAIMemoryExtractionJobRetryResult MessageType = "ai_memory_extraction_job_retry_result"
	TypeAIMemoryDreamRun                 MessageType = "ai_memory_dream_run"
	TypeAIMemoryDreamRunResult           MessageType = "ai_memory_dream_run_result"
	TypeAIMemoryDreamList                MessageType = "ai_memory_dream_list"
	TypeAIMemoryDreamListResult          MessageType = "ai_memory_dream_list_result"
	TypeSubagentDefinitionList           MessageType = "subagent_definition_list"
	TypeSubagentDefinitionListResult     MessageType = "subagent_definition_list_result"
	TypeSubagentDefinitionCreate         MessageType = "subagent_definition_create"
	TypeSubagentDefinitionUpdate         MessageType = "subagent_definition_update"
	TypeSubagentDefinitionDelete         MessageType = "subagent_definition_delete"
	TypeSubagentDefinitionMutationResult MessageType = "subagent_definition_mutation_result"
	TypeSubagentTaskList                 MessageType = "subagent_task_list"
	TypeSubagentTaskListResult           MessageType = "subagent_task_list_result"
	TypeSubagentTaskCheck                MessageType = "subagent_task_check"
	TypeSubagentTaskCancel               MessageType = "subagent_task_cancel"
	TypeSubagentTaskApply                MessageType = "subagent_task_apply"
	TypeSubagentTaskDiscard              MessageType = "subagent_task_discard"
	TypeSubagentTaskResume               MessageType = "subagent_task_resume"
	TypeSubagentTaskResumeResult         MessageType = "subagent_task_resume_result"
	TypeSubagentTaskResult               MessageType = "subagent_task_result"
	TypeSubagentTaskEvent                MessageType = "subagent_task_event"
	TypeFileList                         MessageType = "file_list"
	TypeFileListResult                   MessageType = "file_list_result"
	TypeFileRead                         MessageType = "file_read"
	TypeFileReadResult                   MessageType = "file_read_result"
	TypeChangesList                      MessageType = "changes_list"
	TypeChangesListResult                MessageType = "changes_list_result"
	TypeChangeDiff                       MessageType = "change_diff"
	TypeChangeDiffResult                 MessageType = "change_diff_result"
	TypeChangeRevert                     MessageType = "change_revert"
	TypeChangeRevertResult               MessageType = "change_revert_result"
	TypeHistoryList                      MessageType = "history_list"
	TypeHistoryListResult                MessageType = "history_list_result"
	TypeHistoryDiff                      MessageType = "history_diff"
	TypeHistoryDiffResult                MessageType = "history_diff_result"
	TypeHistoryRevert                    MessageType = "history_revert"
	TypeHistoryRevertResult              MessageType = "history_revert_result"
	TypeError                            MessageType = "error"
	TypeHeartbeat                        MessageType = "heartbeat"
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

type SubagentDefinitionListPayload struct{}

type SubagentDefinitionPayload struct {
	ID             string    `json:"id,omitempty"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	SystemPrompt   string    `json:"system_prompt"`
	ModelID        string    `json:"model_id,omitempty"`
	AllowedTools   []string  `json:"allowed_tools,omitempty"`
	CapabilityMode string    `json:"capability_mode"`
	IsolationMode  string    `json:"isolation_mode"`
	MaxTurns       int       `json:"max_turns"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	Enabled        *bool     `json:"enabled,omitempty"`
	Version        int64     `json:"version,omitempty"`
	Source         string    `json:"source,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type SubagentDefinitionDeletePayload struct {
	DefinitionID string `json:"definition_id"`
}

type SubagentDefinitionListResultPayload struct {
	Definitions []SubagentDefinitionPayload `json:"definitions"`
}

type SubagentDefinitionMutationResultPayload struct {
	Definition  *SubagentDefinitionPayload  `json:"definition,omitempty"`
	Definitions []SubagentDefinitionPayload `json:"definitions,omitempty"`
	DeletedID   string                      `json:"deleted_id,omitempty"`
	Message     string                      `json:"message,omitempty"`
}

type SubagentTaskListPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type SubagentTaskPayload struct {
	TaskID string `json:"task_id"`
}

type SubagentTaskSummary struct {
	ID                string            `json:"id"`
	ParentSessionID   string            `json:"parent_session_id"`
	DefinitionID      string            `json:"definition_id"`
	DefinitionVersion int64             `json:"definition_version"`
	Title             string            `json:"title"`
	Instruction       string            `json:"instruction,omitempty"`
	Status            string            `json:"status"`
	Unread            bool              `json:"unread"`
	Result            string            `json:"result,omitempty"`
	ErrorMessage      string            `json:"error_message,omitempty"`
	ChangeSet         SubagentChangeSet `json:"change_set"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	StartedAt         *time.Time        `json:"started_at,omitempty"`
	CompletedAt       *time.Time        `json:"completed_at,omitempty"`
}

type SubagentFileChange struct {
	Path       string `json:"path"`
	ChangeType string `json:"change_type"`
	BeforeHash string `json:"before_hash,omitempty"`
	AfterHash  string `json:"after_hash,omitempty"`
	BeforeSize int64  `json:"before_size,omitempty"`
	AfterSize  int64  `json:"after_size,omitempty"`
}

type SubagentChangeSet struct {
	WorkspaceID  string               `json:"workspace_id,omitempty"`
	Status       string               `json:"status,omitempty"`
	Files        []SubagentFileChange `json:"files,omitempty"`
	CheckpointID string               `json:"checkpoint_id,omitempty"`
	Message      string               `json:"message,omitempty"`
	CreatedAt    time.Time            `json:"created_at,omitempty"`
	AppliedAt    *time.Time           `json:"applied_at,omitempty"`
	DiscardedAt  *time.Time           `json:"discarded_at,omitempty"`
}

type SubagentTaskListResultPayload struct {
	SessionID string                `json:"session_id"`
	Tasks     []SubagentTaskSummary `json:"tasks"`
}

type SubagentTaskResultPayload struct {
	Task    SubagentTaskSummary `json:"task"`
	Message string              `json:"message,omitempty"`
}

type AgentOnlinePayload struct {
	Status   string `json:"status"`
	BindCode string `json:"bind_code,omitempty"`
}

type UserMessagePayload struct {
	Content string `json:"content"`
}

type AssistantDeltaPayload struct {
	Content   string `json:"content,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

type AssistantDonePayload struct {
	Content   string                               `json:"content"`
	Reasoning string                               `json:"reasoning,omitempty"`
	Usage     TokenUsage                           `json:"usage,omitempty"`
	Context   ContextInfo                          `json:"context,omitempty"`
	Compact   CompactInfo                          `json:"compact,omitempty"`
	Plan      *Plan                                `json:"plan,omitempty"`
	Retrieval *KnowledgeSearchPreviewResultPayload `json:"retrieval,omitempty"`
	Paused    bool                                 `json:"paused,omitempty"`
	Message   string                               `json:"message,omitempty"`
}

type AgentRun struct {
	ID           string     `json:"id"`
	RequestID    string     `json:"request_id,omitempty"`
	SessionID    string     `json:"session_id"`
	Kind         string     `json:"kind"`
	Title        string     `json:"title,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	Status       string     `json:"status"`
	CurrentStep  int        `json:"current_step,omitempty"`
	TotalSteps   int        `json:"total_steps,omitempty"`
	LastSequence int64      `json:"last_sequence,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type AgentRunEvent struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	SessionID    string    `json:"session_id"`
	Sequence     int64     `json:"sequence"`
	Type         string    `json:"type"`
	Title        string    `json:"title,omitempty"`
	Content      string    `json:"content,omitempty"`
	ToolName     string    `json:"tool_name,omitempty"`
	Arguments    string    `json:"arguments,omitempty"`
	Status       string    `json:"status,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	Truncated    bool      `json:"truncated,omitempty"`
	Delta        bool      `json:"delta,omitempty"`
	CurrentStep  int       `json:"current_step,omitempty"`
	TotalSteps   int       `json:"total_steps,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type AgentRunStartedPayload struct {
	Run AgentRun `json:"run"`
}

type AgentRunEventPayload struct {
	Event AgentRunEvent `json:"event"`
}

type AgentRunCompletedPayload struct {
	Run AgentRun `json:"run"`
}

type AgentRunListPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type AgentRunSnapshot struct {
	Run    AgentRun        `json:"run"`
	Events []AgentRunEvent `json:"events"`
}

type AgentRunListResultPayload struct {
	SessionID string             `json:"session_id"`
	Runs      []AgentRunSnapshot `json:"runs"`
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

type ToolResultPayload struct {
	Name         string `json:"name"`
	Arguments    string `json:"arguments,omitempty"`
	Result       string `json:"result"`
	Error        bool   `json:"error,omitempty"`
	Status       string `json:"status,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
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
	AgentMode      string      `json:"agent_mode,omitempty"`
	PermissionMode string      `json:"permission_mode"`
	ContextWindowK int         `json:"context_window_k"`
	Usage          *TokenUsage `json:"usage,omitempty"`
	LastUsage      *TokenUsage `json:"last_usage,omitempty"`
	CurrentPlan    *Plan       `json:"current_plan,omitempty"`
	RAG            RAGSettings `json:"rag"`
	Deleted        bool        `json:"deleted,omitempty"`
	DeletedAt      *time.Time  `json:"deleted_at,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type RAGSettings struct {
	Mode             string   `json:"mode"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
	CategoryIDs      []string `json:"category_ids"`
	TopK             int      `json:"top_k"`
}

type Plan struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"session_id"`
	Goal       string     `json:"goal,omitempty"`
	Status     string     `json:"status"`
	RawContent string     `json:"raw_content,omitempty"`
	Steps      []PlanStep `json:"steps,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type PlanStep struct {
	ID          string `json:"id"`
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
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
	ToolStatus    string     `json:"tool_status,omitempty"`
	ToolErrorCode string     `json:"tool_error_code,omitempty"`
	ToolTruncated bool       `json:"tool_truncated,omitempty"`
	Usage         TokenUsage `json:"usage,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type SessionHistoryResultPayload struct {
	SessionID string                  `json:"session_id"`
	Messages  []SessionHistoryMessage `json:"messages"`
	Count     int                     `json:"count"`
}

type SessionHistoryMetaPayload struct {
	SessionID                 string `json:"session_id,omitempty"`
	LocalMessageCount         int    `json:"local_message_count,omitempty"`
	LocalLastMessageID        string `json:"local_last_message_id,omitempty"`
	LocalLastMessageCreatedAt string `json:"local_last_message_created_at,omitempty"`
	LocalHistoryVersion       int64  `json:"local_history_version,omitempty"`
}

type SessionHistoryMetaResultPayload struct {
	SessionID            string     `json:"session_id"`
	MessageCount         int64      `json:"message_count"`
	LastMessageID        string     `json:"last_message_id,omitempty"`
	LastMessageCreatedAt *time.Time `json:"last_message_created_at,omitempty"`
	HistoryVersion       int64      `json:"history_version"`
	UpToDate             bool       `json:"up_to_date"`
	CanDelta             bool       `json:"can_delta"`
}

type SessionHistoryDeltaPayload struct {
	SessionID      string `json:"session_id,omitempty"`
	AfterMessageID string `json:"after_message_id,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

type SessionHistoryDeltaResultPayload struct {
	SessionID        string                  `json:"session_id"`
	Messages         []SessionHistoryMessage `json:"messages"`
	Count            int                     `json:"count"`
	FullSyncRequired bool                    `json:"full_sync_required,omitempty"`
}

type SessionPermissionSetPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Mode      string `json:"mode"`
}

type SessionModeSetPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Mode      string `json:"mode"`
}

type SessionPlanExecutePayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type SessionPlanExecuteUpdatePayload struct {
	SessionID string `json:"session_id"`
	Plan      *Plan  `json:"plan,omitempty"`
	Message   string `json:"message,omitempty"`
}

type SessionContextSetPayload struct {
	SessionID string `json:"session_id,omitempty"`
	WindowK   int    `json:"window_k"`
}

type SessionContextQueryPayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type SessionRAGSetPayload struct {
	SessionID        string   `json:"session_id,omitempty"`
	Mode             string   `json:"mode"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
	CategoryIDs      []string `json:"category_ids,omitempty"`
	TopK             int      `json:"top_k,omitempty"`
}

// GenerationSettings carries nullable overrides. A null field means inherit
// from the next level instead of setting a numeric zero value.
type GenerationSettings struct {
	Temperature     *float64 `json:"temperature"`
	TopP            *float64 `json:"top_p"`
	MaxOutputTokens *int     `json:"max_output_tokens"`
}

type ResolvedGenerationSettings struct {
	Temperature     float64 `json:"temperature"`
	TopP            float64 `json:"top_p"`
	MaxOutputTokens int     `json:"max_output_tokens"`
}

type SessionGenerationPreferences struct {
	SessionID        string                     `json:"session_id"`
	SessionOverrides GenerationSettings         `json:"session_overrides"`
	ModelDefaults    GenerationSettings         `json:"model_defaults"`
	Effective        ResolvedGenerationSettings `json:"effective"`
	StyleInstruction string                     `json:"style_instruction,omitempty"`
}

type SessionGenerationQueryPayload struct {
	SessionID string `json:"session_id,omitempty"`
}

type SessionGenerationSetPayload struct {
	SessionID string             `json:"session_id,omitempty"`
	Settings  GenerationSettings `json:"settings"`
}

type SessionStyleSetPayload struct {
	SessionID        string `json:"session_id,omitempty"`
	StyleInstruction string `json:"style_instruction"`
}

type SessionGenerationResultPayload struct {
	Preferences SessionGenerationPreferences `json:"preferences"`
	Message     string                       `json:"message,omitempty"`
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

type SessionRegeneratePayload struct {
	SessionID string `json:"session_id,omitempty"`
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
	Summary           string `json:"summary,omitempty"`
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

type SessionContextQueryResultPayload struct {
	SessionID string      `json:"session_id"`
	Context   ContextInfo `json:"context"`
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

type AssetListPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type AssetSummary struct {
	ID          string     `json:"id"`
	SessionID   string     `json:"session_id"`
	RequestID   string     `json:"request_id,omitempty"`
	ToolCallID  string     `json:"tool_call_id,omitempty"`
	ToolName    string     `json:"tool_name,omitempty"`
	Path        string     `json:"path,omitempty"`
	FileName    string     `json:"file_name,omitempty"`
	ContentType string     `json:"content_type,omitempty"`
	Size        int64      `json:"size,omitempty"`
	ShortURL    string     `json:"short_url"`
	Code        string     `json:"code,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type AssetListResultPayload struct {
	SessionID string         `json:"session_id"`
	Assets    []AssetSummary `json:"assets"`
	Count     int            `json:"count"`
}

type KnowledgeCatalogListPayload struct {
	IncludeDeleted bool `json:"include_deleted,omitempty"`
}

type KnowledgeCategory struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ParentID    string     `json:"parent_id,omitempty"`
	AncestorIDs []string   `json:"ancestor_ids,omitempty"`
	SortOrder   int        `json:"sort_order,omitempty"`
	Deleted     bool       `json:"deleted,omitempty"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type KnowledgeBase struct {
	ID                    string     `json:"id"`
	CategoryID            string     `json:"category_id,omitempty"`
	Name                  string     `json:"name"`
	Description           string     `json:"description,omitempty"`
	RAGEnabled            bool       `json:"rag_enabled"`
	ActiveIndexProfileID  string     `json:"active_index_profile_id,omitempty"`
	PendingIndexProfileID string     `json:"pending_index_profile_id,omitempty"`
	Deleted               bool       `json:"deleted,omitempty"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type KnowledgeCatalogResultPayload struct {
	Categories     []KnowledgeCategory `json:"categories"`
	KnowledgeBases []KnowledgeBase     `json:"knowledge_bases"`
	Message        string              `json:"message,omitempty"`
}

type KnowledgeCategoryCreatePayload struct {
	Name      string `json:"name"`
	ParentID  string `json:"parent_id,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}

type KnowledgeCategoryMovePayload struct {
	CategoryID string `json:"category_id"`
	ParentID   string `json:"parent_id,omitempty"`
	SortOrder  int    `json:"sort_order,omitempty"`
}

type KnowledgeCategoryDeletePayload struct {
	CategoryID string `json:"category_id"`
	Reason     string `json:"reason,omitempty"`
	Recursive  bool   `json:"recursive,omitempty"`
}

type KnowledgeBaseCreatePayload struct {
	CategoryID           string `json:"category_id,omitempty"`
	Name                 string `json:"name"`
	Description          string `json:"description,omitempty"`
	RAGEnabled           bool   `json:"rag_enabled"`
	ActiveIndexProfileID string `json:"active_index_profile_id,omitempty"`
}

type KnowledgeBaseUpdatePayload struct {
	KnowledgeBaseID      string `json:"knowledge_base_id"`
	CategoryID           string `json:"category_id,omitempty"`
	Name                 string `json:"name"`
	Description          string `json:"description,omitempty"`
	RAGEnabled           bool   `json:"rag_enabled"`
	ActiveIndexProfileID string `json:"active_index_profile_id,omitempty"`
}

type KnowledgeBaseDeletePayload struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Reason          string `json:"reason,omitempty"`
}

type KnowledgeDocumentListPayload struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	IncludeDeleted  bool   `json:"include_deleted,omitempty"`
}

type KnowledgeDocumentIngestPayload struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	URL             string `json:"url,omitempty"`
	Code            string `json:"code,omitempty"`
}

type KnowledgeDocumentRetryPayload struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	JobID           string `json:"job_id"`
}

type KnowledgeDocumentDeletePayload struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	DocumentID      string `json:"document_id"`
	Reason          string `json:"reason,omitempty"`
}

type KnowledgeDocument struct {
	ID              string     `json:"id"`
	KnowledgeBaseID string     `json:"knowledge_base_id"`
	FileName        string     `json:"file_name"`
	ContentType     string     `json:"content_type,omitempty"`
	Version         int64      `json:"version"`
	Status          string     `json:"status"`
	FailureReason   string     `json:"failure_reason,omitempty"`
	Deleted         bool       `json:"deleted,omitempty"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type KnowledgeIndexingJob struct {
	ID              string     `json:"id"`
	KnowledgeBaseID string     `json:"knowledge_base_id"`
	DocumentID      string     `json:"document_id"`
	IndexProfileID  string     `json:"index_profile_id"`
	Stage           string     `json:"stage"`
	Status          string     `json:"status"`
	TotalChunks     int        `json:"total_chunks"`
	CompletedChunks int        `json:"completed_chunks"`
	FailedChunks    int        `json:"failed_chunks"`
	LastError       string     `json:"last_error,omitempty"`
	RetryCount      int        `json:"retry_count"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

type KnowledgeDocumentListResultPayload struct {
	KnowledgeBaseID string                 `json:"knowledge_base_id"`
	Documents       []KnowledgeDocument    `json:"documents"`
	Jobs            []KnowledgeIndexingJob `json:"jobs"`
	Message         string                 `json:"message,omitempty"`
}

type KnowledgeProfileListPayload struct {
	IncludeDeleted bool `json:"include_deleted,omitempty"`
}

type KnowledgeIndexProfile struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	ParsingProfileID   string     `json:"parsing_profile_id"`
	ChunkingProfileID  string     `json:"chunking_profile_id"`
	EmbeddingProfileID string     `json:"embedding_profile_id"`
	DistanceMetricID   string     `json:"distance_metric_id"`
	Status             string     `json:"status"`
	FailureReason      string     `json:"failure_reason,omitempty"`
	Deleted            bool       `json:"deleted,omitempty"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type KnowledgeProfileListResultPayload struct {
	Profiles []KnowledgeIndexProfile `json:"profiles"`
}

type KnowledgeSearchPreviewPayload struct {
	Query            string   `json:"query"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
	CategoryIDs      []string `json:"category_ids,omitempty"`
	TopK             int      `json:"top_k,omitempty"`
}

type KnowledgeSearchHit struct {
	KnowledgeBaseID string  `json:"knowledge_base_id"`
	DocumentID      string  `json:"document_id"`
	ChunkID         string  `json:"chunk_id"`
	Text            string  `json:"text"`
	SourceName      string  `json:"source_name,omitempty"`
	SourceLocation  string  `json:"source_location,omitempty"`
	Score           float64 `json:"score"`
	Rank            int     `json:"rank"`
	Channel         string  `json:"channel"`
	Origin          string  `json:"origin"`
}

type KnowledgeSearchProfileDiagnostic struct {
	IndexProfileID     string   `json:"index_profile_id"`
	EmbeddingProfileID string   `json:"embedding_profile_id"`
	KnowledgeBaseIDs   []string `json:"knowledge_base_ids"`
	LocalVectorHits    int      `json:"local_vector_hits"`
	LocalKeywordHits   int      `json:"local_keyword_hits"`
	RemoteVectorHits   int      `json:"remote_vector_hits"`
	RemoteFallback     bool     `json:"remote_fallback"`
	CacheFillCount     int      `json:"cache_fill_count"`
	Error              string   `json:"error,omitempty"`
}

type KnowledgeSearchPreviewResultPayload struct {
	Query                    string                             `json:"query"`
	Hits                     []KnowledgeSearchHit               `json:"hits"`
	ResolvedKnowledgeBaseIDs []string                           `json:"resolved_knowledge_base_ids"`
	Profiles                 []KnowledgeSearchProfileDiagnostic `json:"profiles"`
	Warnings                 []string                           `json:"warnings"`
	Error                    string                             `json:"error,omitempty"`
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

type AIMemoryScope struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
}

type AIMemoryContent struct {
	Goal              string `json:"goal"`
	ApplicableContext string `json:"applicable_context,omitempty"`
	Approach          string `json:"approach,omitempty"`
	Result            string `json:"result,omitempty"`
	PainPoints        string `json:"pain_points,omitempty"`
	RootCause         string `json:"root_cause,omitempty"`
	Lessons           string `json:"lessons,omitempty"`
	Verification      string `json:"verification,omitempty"`
}

type AIMemorySource struct {
	Type       string    `json:"type"`
	SessionID  string    `json:"session_id,omitempty"`
	AgentRunID string    `json:"agent_run_id,omitempty"`
	EventIDs   []string  `json:"event_ids,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type AIMemoryRevision struct {
	ID         string           `json:"id"`
	Version    int              `json:"version"`
	Content    AIMemoryContent  `json:"content"`
	Confidence float64          `json:"confidence"`
	Author     string           `json:"author"`
	Sources    []AIMemorySource `json:"sources"`
	CreatedAt  time.Time        `json:"created_at"`
}

type AIMemory struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Kind           string             `json:"kind"`
	Scope          AIMemoryScope      `json:"scope"`
	Tags           []string           `json:"tags"`
	Status         string             `json:"status"`
	CurrentVersion int                `json:"current_version"`
	Revisions      []AIMemoryRevision `json:"revisions"`
	HumanLocked    bool               `json:"human_locked"`
	SupersedesID   string             `json:"supersedes_id,omitempty"`
	UseCount       int64              `json:"use_count"`
	LastUsedAt     *time.Time         `json:"last_used_at,omitempty"`
	DeletedAt      *time.Time         `json:"deleted_at,omitempty"`
	DeletionReason string             `json:"deletion_reason,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

type AIMemoryCandidate struct {
	ID             string           `json:"id"`
	Title          string           `json:"title"`
	Kind           string           `json:"kind"`
	Scope          AIMemoryScope    `json:"scope"`
	Tags           []string         `json:"tags"`
	Content        AIMemoryContent  `json:"content"`
	Confidence     float64          `json:"confidence"`
	Sources        []AIMemorySource `json:"sources"`
	Status         string           `json:"status"`
	TargetMemoryID string           `json:"target_memory_id,omitempty"`
	ReviewNote     string           `json:"review_note,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type AIMemoryInput struct {
	Title      string          `json:"title"`
	Kind       string          `json:"kind"`
	Scope      AIMemoryScope   `json:"scope"`
	Tags       []string        `json:"tags,omitempty"`
	Content    AIMemoryContent `json:"content"`
	Confidence float64         `json:"confidence,omitempty"`
}

type AIMemoryListPayload struct {
	Text           string   `json:"text,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Kinds          []string `json:"kinds,omitempty"`
	Statuses       []string `json:"statuses,omitempty"`
	ScopeTypes     []string `json:"scope_types,omitempty"`
	ScopeKey       string   `json:"scope_key,omitempty"`
	IncludeDeleted bool     `json:"include_deleted,omitempty"`
	Limit          int      `json:"limit,omitempty"`
}

type AIMemoryListResultPayload struct {
	Memories []AIMemory `json:"memories"`
	Message  string     `json:"message,omitempty"`
}

type AIMemoryCreatePayload struct {
	Memory AIMemoryInput `json:"memory"`
}

type AIMemoryUpdatePayload struct {
	MemoryID string        `json:"memory_id"`
	Memory   AIMemoryInput `json:"memory"`
}

type AIMemoryDeletePayload struct {
	MemoryID string `json:"memory_id"`
	Reason   string `json:"reason,omitempty"`
}

type AIMemoryRestorePayload struct {
	MemoryID string `json:"memory_id"`
}

type AIMemoryCandidateListPayload struct {
	Statuses []string `json:"statuses,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type AIMemoryCandidateListResultPayload struct {
	Candidates []AIMemoryCandidate `json:"candidates"`
	Message    string              `json:"message,omitempty"`
}

type AIMemoryCandidateApprovePayload struct {
	CandidateID string `json:"candidate_id"`
	MemoryID    string `json:"memory_id,omitempty"`
}

type AIMemoryCandidateRejectPayload struct {
	CandidateID string `json:"candidate_id"`
	Note        string `json:"note,omitempty"`
}

type AIMemoryCandidateMutationResultPayload struct {
	Memories   []AIMemory          `json:"memories"`
	Candidates []AIMemoryCandidate `json:"candidates"`
	Message    string              `json:"message,omitempty"`
}

type AIMemoryExtractionJob struct {
	ID               string     `json:"id"`
	AgentRunID       string     `json:"agent_run_id"`
	ExtractorVersion string     `json:"extractor_version"`
	Status           string     `json:"status"`
	Attempts         int        `json:"attempts"`
	LastError        string     `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type AIMemoryExtractionJobListPayload struct {
	Statuses []string `json:"statuses,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type AIMemoryExtractionJobListResultPayload struct {
	Jobs    []AIMemoryExtractionJob `json:"jobs"`
	Message string                  `json:"message,omitempty"`
}

type AIMemoryExtractionJobRetryPayload struct {
	JobID string `json:"job_id"`
}

type AIMemoryDreamAction struct {
	CandidateID    string `json:"candidate_id,omitempty"`
	CandidateTitle string `json:"candidate_title,omitempty"`
	MemoryID       string `json:"memory_id,omitempty"`
	MemoryTitle    string `json:"memory_title,omitempty"`
	Decision       string `json:"decision"`
	Reason         string `json:"reason,omitempty"`
	Applied        bool   `json:"applied"`
	FailureReason  string `json:"failure_reason,omitempty"`
}

type AIMemoryDreamRun struct {
	ID              string                `json:"id"`
	Status          string                `json:"status"`
	Trigger         string                `json:"trigger"`
	CandidateCount  int                   `json:"candidate_count"`
	CreatedCount    int                   `json:"created_count"`
	MergedCount     int                   `json:"merged_count"`
	SupersededCount int                   `json:"superseded_count"`
	RejectedCount   int                   `json:"rejected_count"`
	Actions         []AIMemoryDreamAction `json:"actions"`
	LastError       string                `json:"last_error,omitempty"`
	StartedAt       time.Time             `json:"started_at"`
	FinishedAt      *time.Time            `json:"finished_at,omitempty"`
}

type AIMemoryDreamRunPayload struct {
	CandidateLimit int `json:"candidate_limit,omitempty"`
	MemoryLimit    int `json:"memory_limit,omitempty"`
}

type AIMemoryDreamListPayload struct {
	Limit int `json:"limit,omitempty"`
}

type AIMemoryDreamResultPayload struct {
	Runs       []AIMemoryDreamRun  `json:"runs"`
	Memories   []AIMemory          `json:"memories,omitempty"`
	Candidates []AIMemoryCandidate `json:"candidates,omitempty"`
	Message    string              `json:"message,omitempty"`
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
