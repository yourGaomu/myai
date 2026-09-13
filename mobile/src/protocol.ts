// MessageType 必须与 Go 的 core/remote/protocol 保持同步；请求、流式事件和终态响应共用同一信封。
export type MessageType =
  | "heartbeat"
  | "user_message"
  | "assistant_delta"
  | "assistant_done"
  | "agent_run_started"
  | "agent_run_event"
  | "agent_run_completed"
  | "agent_run_list"
  | "agent_run_list_result"
  | "tool_call"
  | "tool_result"
  | "permission_ask"
  | "permission_result"
  | "session_list"
  | "session_list_result"
  | "session_new"
  | "session_load"
  | "session_delete"
  | "session_delete_result"
  | "session_restore"
  | "session_restore_result"
  | "session_changed"
  | "session_history"
  | "session_history_result"
  | "session_history_meta"
  | "session_history_meta_result"
  | "session_history_delta"
  | "session_history_delta_result"
  | "session_permission_set"
  | "session_permission_set_result"
  | "session_mode_set"
  | "session_mode_set_result"
  | "session_plan_execute"
  | "session_plan_update"
  | "session_plan_execute_result"
  | "session_context_query"
  | "session_context_query_result"
  | "session_context_set"
  | "session_context_set_result"
  | "session_rag_set"
  | "session_rag_set_result"
  | "session_generation_query"
  | "session_generation_query_result"
  | "session_generation_set"
  | "session_generation_set_result"
  | "session_style_set"
  | "session_style_set_result"
  | "session_compact"
  | "session_compact_result"
  | "session_pause"
  | "session_pause_result"
  | "session_regenerate"
  | "model_list"
  | "model_list_result"
  | "model_switch"
  | "model_switch_result"
  | "model_config_add"
  | "model_config_add_result"
  | "model_config_test"
  | "model_config_test_result"
  | "model_config_update"
  | "model_config_delete"
  | "model_config_enabled_set"
  | "model_config_default_set"
  | "model_config_mutation_result"
  | "skill_list"
  | "skill_list_result"
  | "skill_reload"
  | "skill_reload_result"
  | "plugin_list"
  | "plugin_list_result"
  | "plugin_reload"
  | "plugin_reload_result"
  | "plugin_enable"
  | "plugin_disable"
  | "plugin_mutation_result"
  | "asset_list"
  | "asset_list_result"
  | "knowledge_catalog_list"
  | "knowledge_catalog_list_result"
  | "knowledge_category_create"
  | "knowledge_category_move"
  | "knowledge_category_delete"
  | "knowledge_base_create"
  | "knowledge_base_update"
  | "knowledge_base_delete"
  | "knowledge_catalog_mutation_result"
  | "knowledge_document_list"
  | "knowledge_document_list_result"
  | "knowledge_document_ingest"
  | "knowledge_document_retry"
  | "knowledge_document_delete"
  | "knowledge_document_mutation_result"
  | "knowledge_profile_list"
  | "knowledge_profile_list_result"
  | "knowledge_search_preview"
  | "knowledge_search_preview_result"
  | "ai_memory_list"
  | "ai_memory_list_result"
  | "ai_memory_create"
  | "ai_memory_update"
  | "ai_memory_delete"
  | "ai_memory_restore"
  | "ai_memory_mutation_result"
  | "ai_memory_candidate_list"
  | "ai_memory_candidate_list_result"
  | "ai_memory_candidate_approve"
  | "ai_memory_candidate_reject"
  | "ai_memory_candidate_mutation_result"
  | "ai_memory_extraction_job_list"
  | "ai_memory_extraction_job_list_result"
  | "ai_memory_extraction_job_retry"
  | "ai_memory_extraction_job_retry_result"
  | "ai_memory_dream_run"
  | "ai_memory_dream_run_result"
  | "ai_memory_dream_list"
  | "ai_memory_dream_list_result"
  | "subagent_definition_list"
  | "subagent_definition_list_result"
  | "subagent_definition_create"
  | "subagent_definition_update"
  | "subagent_definition_delete"
  | "subagent_definition_mutation_result"
  | "subagent_task_list"
  | "subagent_task_list_result"
  | "subagent_task_check"
  | "subagent_task_message"
  | "subagent_task_followup"
  | "subagent_task_wait"
  | "subagent_task_wait_result"
  | "subagent_task_cancel"
  | "subagent_task_apply"
  | "subagent_task_discard"
  | "subagent_task_resume"
  | "subagent_task_resume_result"
  | "subagent_task_result"
  | "subagent_task_event"
  | "file_list"
  | "file_list_result"
  | "file_read"
  | "file_read_result"
  | "changes_list"
  | "changes_list_result"
  | "change_diff"
  | "change_diff_result"
  | "change_revert"
  | "change_revert_result"
  | "history_list"
  | "history_list_result"
  | "history_diff"
  | "history_diff_result"
  | "history_revert"
  | "history_revert_result"
  | "error";

// RelayMessage 负责跨端路由，具体业务字段放在 payload DTO 中，避免不同消息重复定义公共字段。
export type RelayMessage<TPayload = unknown> = {
  type: MessageType;
  request_id?: string;
  user_id?: string;
  device_id?: string;
  session_id?: string;
  client_token?: string;
  payload?: TPayload;
};

export type SubagentDefinition = {
  id: string;
  name: string;
  description?: string;
  system_prompt: string;
  model_id?: string;
  allowed_tools?: string[];
  capability_mode: "read_only" | "read_write" | "execute" | "all";
  isolation_mode: "direct" | "snapshot" | "git_worktree" | "opensandbox";
  max_turns: number;
  timeout_seconds: number;
  enabled: boolean;
  version?: number;
  source?: "builtin" | "user";
  created_at?: string;
  updated_at?: string;
};

export type SubagentDefinitionListResultPayload = {
  definitions?: SubagentDefinition[];
};

export type SubagentDefinitionMutationResultPayload = {
  definition?: SubagentDefinition;
  definitions?: SubagentDefinition[];
  deleted_id?: string;
  message?: string;
};

export type SubagentFileChange = {
  path: string;
  change_type: "added" | "modified" | "deleted";
  before_hash?: string;
  after_hash?: string;
  before_size?: number;
  after_size?: number;
};

export type SubagentChangeSet = {
  workspace_id?: string;
  status?: "none" | "pending" | "applied" | "discarded" | "conflict";
  files?: SubagentFileChange[];
  checkpoint_id?: string;
  message?: string;
  created_at?: string;
  applied_at?: string;
  discarded_at?: string;
};

export type SubagentTask = {
  id: string;
  parent_session_id: string;
  parent_task_id?: string;
  parent_run_id?: string;
  plan_id?: string;
  step_id?: string;
  agent_path?: string;
  agent_nickname?: string;
  definition_id: string;
  definition_version: number;
  title: string;
  instruction?: string;
  status: "queued" | "running" | "waiting_subagents" | "waiting_permission" | "succeeded" | "failed" | "canceled";
  unread: boolean;
  result?: string;
  error_message?: string;
  change_set: SubagentChangeSet;
  can_followup?: boolean;
  created_at: string;
  updated_at: string;
  started_at?: string;
  completed_at?: string;
  event_sequence?: number;
  event_kind?: string;
};

export type SubagentTaskListResultPayload = {
  session_id: string;
  tasks?: SubagentTask[];
};

export type SubagentTaskResultPayload = {
  task: SubagentTask;
  message?: string;
  timed_out?: boolean;
  sequence?: number;
  kind?: string;
  emitted_at?: string;
  run_id?: string;
  content?: string;
  tool_name?: string;
  arguments?: string;
  status?: string;
  error_code?: string;
  error_message?: string;
  truncated?: boolean;
  delta?: boolean;
};

export type SubagentTaskEvent = {
  task_id: string;
  sequence: number;
  kind: string;
  run_id?: string;
  content?: string;
  tool_name?: string;
  arguments?: string;
  status?: string;
  error_code?: string;
  error_message?: string;
  truncated?: boolean;
  delta?: boolean;
  emitted_at?: string;
};

export type SubagentTaskWaitResultPayload = {
  session_id: string;
  task: SubagentTask;
  timed_out: boolean;
  woken_by_mailbox?: boolean;
  sequence?: number;
};

export type PairResponse = {
  user_id: string;
  device_id: string;
  client_token: string;
};

export type AssistantDeltaPayload = {
  content?: string;
  reasoning?: string;
};

export type AssistantDonePayload = {
  content?: string;
  reasoning?: string;
  usage?: TokenUsage;
  context?: ContextInfo;
  compact?: CompactInfo;
  plan?: Plan;
  retrieval?: KnowledgeSearchPreviewResultPayload;
  paused?: boolean;
  message?: string;
};

export type AgentRunStatus = "running" | "succeeded" | "failed" | "paused" | "canceled";
export type AgentRunKind = "chat" | "regenerate" | "plan" | "internal";
export type AgentRunEventType =
  | "progress"
  | "reasoning"
  | "tool_call"
  | "tool_result"
  | "permission"
  | "plan_update"
  | "completed"
  | "paused"
  | "failed"
  | "canceled";

export type AgentRun = {
  id: string;
  request_id?: string;
  session_id: string;
  parent_run_id?: string;
  plan_id?: string;
  step_id?: string;
  task_id?: string;
  kind: AgentRunKind;
  title?: string;
  reason?: string;
  status: AgentRunStatus;
  current_step?: number;
  total_steps?: number;
  last_sequence?: number;
  error_message?: string;
  started_at: string;
  finished_at?: string;
};

export type AgentRunEvent = {
  id: string;
  run_id: string;
  session_id: string;
  sequence: number;
  type: AgentRunEventType;
  title?: string;
  content?: string;
  tool_name?: string;
  arguments?: string;
  status?: string;
  error_code?: string;
  error_message?: string;
  truncated?: boolean;
  delta?: boolean;
  current_step?: number;
  total_steps?: number;
  created_at: string;
};

export type AgentRunSnapshot = {
  run: AgentRun;
  events: AgentRunEvent[];
};

export type AgentRunStartedPayload = { run: AgentRun };
export type AgentRunEventPayload = { event: AgentRunEvent };
export type AgentRunCompletedPayload = { run: AgentRun };
export type AgentRunListResultPayload = {
  session_id: string;
  runs?: AgentRunSnapshot[];
};

export type TokenUsage = {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  reasoning_tokens?: number;
  prompt_cached_tokens?: number;
  available?: boolean;
};

export type ErrorPayload = {
  message?: string;
};

export type ToolCallPayload = {
  name?: string;
  arguments?: string;
};

export type ToolResultPayload = {
  name?: string;
  arguments?: string;
  result?: string;
  error?: boolean;
  status?: "success" | "failed" | "denied" | "timeout" | "canceled" | string;
  error_code?: string;
  error_message?: string;
  truncated?: boolean;
};

export type PermissionAskPayload = {
  name?: string;
  arguments?: string;
  permission?: string;
};

export type PermissionResultPayload = {
  allowed: boolean;
};

export type SessionSummary = {
  id: string;
  title?: string;
  model?: string;
  agent_mode?: string;
  permission_mode?: string;
  context_window_k?: number;
  usage?: TokenUsage;
  last_usage?: TokenUsage;
  current_plan?: Plan;
  rag?: RAGSettings;
  deleted?: boolean;
  deleted_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type RAGSettings = {
  mode: "off" | "manual" | "auto" | "always" | string;
  knowledge_base_ids: string[];
  category_ids: string[];
  top_k: number;
};

export type SessionRAGSetPayload = RAGSettings & {
  session_id?: string;
};

export type Plan = {
  id: string;
  session_id: string;
  goal?: string;
  status: string;
  revision?: number;
  raw_content?: string;
  steps?: PlanStep[];
  created_at?: string;
  updated_at?: string;
};

export type PlanStep = {
  id: string;
  order: number;
  title: string;
  description?: string;
  dependencies?: string[];
  status: string;
  retry_count?: number;
  max_retries?: number;
  last_error?: string;
  agent_task_id?: string;
  started_at?: string;
  completed_at?: string;
};

export type SessionListResultPayload = {
  current_session_id?: string;
  sessions?: SessionSummary[];
  include_deleted?: boolean;
};

export type SessionChangedPayload = SessionListResultPayload & {
  session?: SessionSummary;
};

export type SessionHistoryPayload = {
  session_id?: string;
};

export type SessionDeletePayload = {
  session_id: string;
};

export type SessionRestorePayload = {
  session_id: string;
};

export type SessionHistoryMessage = {
  id: string;
  role: "user" | "assistant" | "tool_call" | "tool" | string;
  content?: string;
  reasoning?: string;
  tool_call_id?: string;
  tool_name?: string;
  tool_arguments?: string;
  tool_error?: string;
  tool_status?: string;
  tool_error_code?: string;
  tool_truncated?: boolean;
  usage?: TokenUsage;
  created_at?: string;
};

export type SessionHistoryResultPayload = {
  session_id: string;
  messages?: SessionHistoryMessage[];
  count?: number;
};

export type SessionHistoryMetaPayload = {
  session_id?: string;
  local_message_count?: number;
  local_last_message_id?: string;
  local_last_message_created_at?: string;
  local_history_version?: number;
};

export type SessionHistoryMetaResultPayload = {
  session_id: string;
  message_count?: number;
  last_message_id?: string;
  last_message_created_at?: string;
  history_version?: number;
  up_to_date?: boolean;
  can_delta?: boolean;
};

export type SessionHistoryDeltaPayload = {
  session_id?: string;
  after_message_id?: string;
  limit?: number;
};

export type SessionHistoryDeltaResultPayload = {
  session_id: string;
  messages?: SessionHistoryMessage[];
  count?: number;
  full_sync_required?: boolean;
};

export type SessionPermissionSetPayload = {
  session_id?: string;
  mode: string;
};

export type SessionModeSetPayload = {
  session_id?: string;
  mode: string;
};

export type SessionPlanExecutePayload = {
  session_id?: string;
};

export type SessionPlanExecuteUpdatePayload = {
  session_id: string;
  plan?: Plan;
  message?: string;
};

export type SessionContextSetPayload = {
  session_id?: string;
  window_k: number;
};

export type SessionContextQueryPayload = {
  session_id?: string;
};

export type GenerationSettings = {
  temperature: number | null;
  top_p: number | null;
  max_output_tokens: number | null;
};

export type ResolvedGenerationSettings = {
  temperature: number;
  top_p: number;
  max_output_tokens: number;
};

export type SessionGenerationPreferences = {
  session_id: string;
  session_overrides: GenerationSettings;
  model_defaults: GenerationSettings;
  effective: ResolvedGenerationSettings;
  style_instruction?: string;
};

export type SessionGenerationQueryPayload = {
  session_id?: string;
};

export type SessionGenerationSetPayload = {
  session_id?: string;
  settings: GenerationSettings;
};

export type SessionStyleSetPayload = {
  session_id?: string;
  style_instruction: string;
};

export type SessionGenerationResultPayload = {
  preferences?: SessionGenerationPreferences;
  message?: string;
};

export type SessionCompactPayload = {
  session_id?: string;
};

export type SessionPausePayload = {
  session_id?: string;
};

export type SessionPauseResultPayload = {
  session_id: string;
  paused?: boolean;
  message?: string;
};

export type SessionRegeneratePayload = {
  session_id?: string;
};

export type ContextInfo = {
  window_k?: number;
  full_tokens?: number;
  selected_tokens?: number;
  summary_tokens?: number;
  prefix_tokens?: number;
  cacheable_tokens?: number;
  full_messages?: number;
  selected_messages?: number;
  compacted_messages?: number;
  has_summary?: boolean;
  truncated?: boolean;
  summary_version?: number;
  summary_hash?: string;
  prefix_hash?: string;
  summary?: string;
  checkpoint?: CompactionCheckpoint;
};

export type CompactionSummary = {
  current_goal?: string;
  preferences?: string[];
  constraints?: string[];
  decisions?: string[];
  completed_work?: string[];
  modified_files?: string[];
  tool_verification?: string[];
  problems?: string[];
  open_tasks?: string[];
  next_steps?: string[];
  references?: string[];
};

export type CompactionCheckpoint = {
  version: number;
  source_start_message: number;
  source_end_message: number;
  source_history_hash?: string;
  summary: string;
  summary_data?: CompactionSummary;
  created_at?: string;
};

export type CompactInfo = {
  triggered?: boolean;
  reason?: string;
  before_tokens?: number;
  after_tokens?: number;
  new_messages?: number;
  compacted_messages?: number;
  summary_tokens?: number;
  summary_version?: number;
  summary_hash?: string;
  prefix_hash?: string;
  cacheable_tokens?: number;
  checkpoint?: CompactionCheckpoint;
};

export type SessionSettingsResultPayload = {
  current_session_id?: string;
  session?: SessionSummary;
  sessions?: SessionSummary[];
  context?: ContextInfo;
  message?: string;
};

export type SessionContextQueryResultPayload = {
  session_id: string;
  context?: ContextInfo;
};

export type ModelSummary = {
  id: string;
  name?: string;
  provider?: string;
  protocol?: string;
  auth_type?: string;
  base_url?: string;
  has_api_key?: boolean;
  model_name?: string;
  enabled?: boolean;
  is_default?: boolean;
  defaults?: GenerationSettings;
};

export type ModelListResultPayload = {
  current_model_id?: string;
  models?: ModelSummary[];
};

export type ModelSwitchPayload = {
  model_id: string;
};

export type ModelSwitchResultPayload = {
  current_model_id?: string;
  models?: ModelSummary[];
  session?: SessionSummary;
  message?: string;
};

export type ModelConfigAddPayload = {
  id: string;
  name?: string;
  provider?: string;
  protocol?: string;
  auth_type?: string;
  base_url?: string;
  api_key?: string;
  model_name?: string;
  is_default?: boolean;
  defaults?: GenerationSettings;
};

export type ModelConfigAddResultPayload = {
  model?: ModelSummary;
  models?: ModelSummary[];
  message?: string;
};

export type ModelConfigTestResultPayload = {
  success?: boolean;
  latency_ms?: number;
  message?: string;
};

export type ModelConfigUpdatePayload = ModelConfigAddPayload;

export type ModelConfigDeletePayload = {
  id: string;
};

export type ModelConfigEnabledSetPayload = {
  id: string;
  enabled: boolean;
};

export type ModelConfigDefaultSetPayload = {
  id: string;
};

export type ModelConfigMutationResultPayload = {
  model?: ModelSummary;
  deleted_id?: string;
  models?: ModelSummary[];
  message?: string;
};

export type SkillSummary = {
  name: string;
  description?: string;
  path?: string;
  triggers?: string[];
  updated_at?: string;
};

export type SkillListResultPayload = {
  root?: string;
  skills?: SkillSummary[];
  count?: number;
  reloaded?: boolean;
  message?: string;
};

export type PluginInfo = {
  id: string;
  name: string;
  version: string;
  protocol: string;
  entrypoint?: string;
  directory?: string;
  status: "loaded" | "disabled" | "failed" | string;
  error?: string;
  enabled: boolean;
  required: boolean;
};

export type PluginListResultPayload = {
  root?: string;
  plugins?: PluginInfo[];
  count?: number;
  reloaded?: boolean;
  message?: string;
};

export type PluginTogglePayload = {
  plugin_id: string;
};

export type PluginMutationResultPayload = {
  plugin_id?: string;
  enabled?: boolean;
  plugins?: PluginInfo[];
  count?: number;
  message?: string;
};

export type AssetListPayload = {
  session_id?: string;
  limit?: number;
};

export type AssetSummary = {
  id: string;
  session_id?: string;
  request_id?: string;
  tool_call_id?: string;
  tool_name?: string;
  path?: string;
  file_name?: string;
  content_type?: string;
  size?: number;
  short_url: string;
  code?: string;
  expires_at?: string;
  created_at?: string;
};

export type UploadedAssetPayload = {
  code: string;
  short_url: string;
  bucket?: string;
  object_key?: string;
  file_name: string;
  content_type?: string;
  size?: number;
  expires_at?: string;
};

export type AssetListResultPayload = {
  session_id: string;
  assets?: AssetSummary[];
  count?: number;
};

export type KnowledgeCategory = {
  id: string;
  name: string;
  parent_id?: string;
  ancestor_ids?: string[];
  sort_order?: number;
  deleted?: boolean;
  deleted_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type KnowledgeBase = {
  id: string;
  category_id?: string;
  name: string;
  description?: string;
  rag_enabled: boolean;
  active_index_profile_id?: string;
  pending_index_profile_id?: string;
  deleted?: boolean;
  deleted_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type KnowledgeCatalogResultPayload = {
  categories: KnowledgeCategory[];
  knowledge_bases: KnowledgeBase[];
  message?: string;
};

export type KnowledgeCatalogListPayload = {
  include_deleted?: boolean;
};

export type KnowledgeCategoryCreatePayload = {
  name: string;
  parent_id?: string;
  sort_order?: number;
};

export type KnowledgeCategoryMovePayload = {
  category_id: string;
  parent_id?: string;
  sort_order?: number;
};

export type KnowledgeCategoryDeletePayload = {
  category_id: string;
  reason?: string;
  recursive?: boolean;
};

export type KnowledgeBaseCreatePayload = {
  category_id?: string;
  name: string;
  description?: string;
  rag_enabled: boolean;
  active_index_profile_id?: string;
};

export type KnowledgeBaseUpdatePayload = {
  knowledge_base_id: string;
  category_id?: string;
  name: string;
  description?: string;
  rag_enabled: boolean;
  active_index_profile_id?: string;
};

export type KnowledgeBaseDeletePayload = {
  knowledge_base_id: string;
  reason?: string;
};

export type KnowledgeDocumentListPayload = {
  knowledge_base_id: string;
  include_deleted?: boolean;
};

export type KnowledgeDocumentIngestPayload = {
  knowledge_base_id: string;
  url?: string;
  code?: string;
};

export type KnowledgeDocumentRetryPayload = {
  knowledge_base_id: string;
  job_id: string;
};

export type KnowledgeDocumentDeletePayload = {
  knowledge_base_id: string;
  document_id: string;
  reason?: string;
};

export type KnowledgeDocument = {
  id: string;
  knowledge_base_id: string;
  file_name: string;
  content_type?: string;
  version: number;
  status: "uploaded" | "parsing" | "chunking" | "embedding" | "indexing" | "ready" | "failed" | "deleted" | string;
  failure_reason?: string;
  deleted?: boolean;
  deleted_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type KnowledgeIndexingJob = {
  id: string;
  knowledge_base_id: string;
  document_id: string;
  index_profile_id: string;
  stage: string;
  status: string;
  total_chunks: number;
  completed_chunks: number;
  failed_chunks: number;
  last_error?: string;
  retry_count: number;
  created_at?: string;
  updated_at?: string;
  completed_at?: string;
};

export type KnowledgeDocumentListResultPayload = {
  knowledge_base_id: string;
  documents: KnowledgeDocument[];
  jobs: KnowledgeIndexingJob[];
  message?: string;
};

export type KnowledgeIndexProfile = {
  id: string;
  name: string;
  parsing_profile_id: string;
  chunking_profile_id: string;
  embedding_profile_id: string;
  distance_metric_id: string;
  status: string;
  failure_reason?: string;
  deleted?: boolean;
  deleted_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type KnowledgeProfileListResultPayload = {
  profiles: KnowledgeIndexProfile[];
};

export type KnowledgeProfileListPayload = {
  include_deleted?: boolean;
};

export type KnowledgeSearchHit = {
  knowledge_base_id: string;
  document_id: string;
  chunk_id: string;
  text: string;
  source_name?: string;
  source_location?: string;
  score: number;
  rank: number;
  channel: string;
  origin: string;
};

export type KnowledgeSearchProfileDiagnostic = {
  index_profile_id: string;
  embedding_profile_id: string;
  knowledge_base_ids: string[];
  local_vector_hits: number;
  local_keyword_hits: number;
  remote_vector_hits: number;
  remote_fallback: boolean;
  cache_fill_count: number;
  error?: string;
};

export type KnowledgeSearchPreviewResultPayload = {
  query: string;
  hits: KnowledgeSearchHit[];
  resolved_knowledge_base_ids: string[];
  profiles: KnowledgeSearchProfileDiagnostic[];
  warnings: string[];
  error?: string;
};

export type KnowledgeSearchPreviewPayload = {
  query: string;
  knowledge_base_ids?: string[];
  category_ids?: string[];
  top_k?: number;
};

export type AIMemoryKind = "experience" | "failure" | "decision" | "preference";
export type AIMemoryStatus = "active" | "archived" | "superseded" | "deleted";
export type AIMemoryCandidateStatus = "pending" | "approved" | "rejected" | "merged";

export type AIMemoryScope = {
  type: "global" | "workspace" | "project" | "session";
  key?: string;
};

export type AIMemoryContent = {
  goal: string;
  applicable_context?: string;
  approach?: string;
  result?: string;
  pain_points?: string;
  root_cause?: string;
  lessons?: string;
  verification?: string;
};

export type AIMemorySource = {
  type: "agent_run" | "session" | "manual" | "dream";
  session_id?: string;
  agent_run_id?: string;
  event_ids?: string[];
  created_at: string;
};

export type AIMemoryRevision = {
  id: string;
  version: number;
  content: AIMemoryContent;
  confidence: number;
  author: "model" | "human" | "system";
  sources: AIMemorySource[];
  created_at: string;
};

export type AIMemory = {
  id: string;
  title: string;
  kind: AIMemoryKind;
  scope: AIMemoryScope;
  tags: string[];
  status: AIMemoryStatus;
  current_version: number;
  revisions: AIMemoryRevision[];
  human_locked: boolean;
  supersedes_id?: string;
  use_count: number;
  last_used_at?: string;
  deleted_at?: string;
  deletion_reason?: string;
  created_at: string;
  updated_at: string;
};

export type AIMemoryCandidate = {
  id: string;
  title: string;
  kind: AIMemoryKind;
  scope: AIMemoryScope;
  tags: string[];
  content: AIMemoryContent;
  confidence: number;
  sources: AIMemorySource[];
  status: AIMemoryCandidateStatus;
  target_memory_id?: string;
  review_note?: string;
  created_at: string;
  updated_at: string;
};

export type AIMemoryInput = {
  title: string;
  kind: AIMemoryKind;
  scope: AIMemoryScope;
  tags?: string[];
  content: AIMemoryContent;
  confidence?: number;
};

export type AIMemoryListPayload = {
  text?: string;
  tags?: string[];
  kinds?: AIMemoryKind[];
  statuses?: AIMemoryStatus[];
  scope_types?: AIMemoryScope["type"][];
  scope_key?: string;
  include_deleted?: boolean;
  limit?: number;
};

export type AIMemoryListResultPayload = {
  memories: AIMemory[];
  message?: string;
};

export type AIMemoryCreatePayload = { memory: AIMemoryInput };
export type AIMemoryUpdatePayload = { memory_id: string; memory: AIMemoryInput };
export type AIMemoryDeletePayload = { memory_id: string; reason?: string };
export type AIMemoryRestorePayload = { memory_id: string };

export type AIMemoryCandidateListPayload = {
  statuses?: AIMemoryCandidateStatus[];
  limit?: number;
};

export type AIMemoryCandidateListResultPayload = {
  candidates: AIMemoryCandidate[];
  message?: string;
};

export type AIMemoryCandidateApprovePayload = { candidate_id: string; memory_id?: string };
export type AIMemoryCandidateRejectPayload = { candidate_id: string; note?: string };

export type AIMemoryCandidateMutationResultPayload = {
  memories: AIMemory[];
  candidates: AIMemoryCandidate[];
  message?: string;
};

export type AIMemoryExtractionJobStatus = "pending" | "running" | "succeeded" | "failed";

export type AIMemoryExtractionJob = {
  id: string;
  agent_run_id: string;
  extractor_version: string;
  status: AIMemoryExtractionJobStatus;
  attempts: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
};

export type AIMemoryExtractionJobListPayload = {
  statuses?: AIMemoryExtractionJobStatus[];
  limit?: number;
};

export type AIMemoryExtractionJobListResultPayload = {
  jobs: AIMemoryExtractionJob[];
  message?: string;
};

export type AIMemoryExtractionJobRetryPayload = { job_id: string };

export type AIMemoryDreamDecision = "create" | "merge" | "supersede" | "keep_both" | "reject" | "needs_review";

export type AIMemoryDreamAction = {
  candidate_id?: string;
  candidate_title?: string;
  memory_id?: string;
  memory_title?: string;
  decision: AIMemoryDreamDecision;
  reason?: string;
  applied: boolean;
  failure_reason?: string;
};

export type AIMemoryDreamRun = {
  id: string;
  status: "pending" | "running" | "succeeded" | "failed";
  trigger: string;
  candidate_count: number;
  created_count: number;
  merged_count: number;
  superseded_count: number;
  rejected_count: number;
  actions: AIMemoryDreamAction[];
  last_error?: string;
  started_at: string;
  finished_at?: string;
};

export type AIMemoryDreamRunPayload = { candidate_limit?: number; memory_limit?: number };
export type AIMemoryDreamListPayload = { limit?: number };
export type AIMemoryDreamResultPayload = {
  runs: AIMemoryDreamRun[];
  memories?: AIMemory[];
  candidates?: AIMemoryCandidate[];
  message?: string;
};

export type FileListPayload = {
  path: string;
  include_hidden?: boolean;
  limit?: number;
};

export type FileEntry = {
  path: string;
  name: string;
  type: "file" | "dir";
  size?: number;
  modified_at?: string;
};

export type FileListResultPayload = {
  path: string;
  parent?: string;
  entries?: FileEntry[];
  count?: number;
  truncated?: boolean;
};

export type FileReadPayload = {
  path: string;
};

export type FileReadResultPayload = {
  path: string;
  name: string;
  language: string;
  content?: string;
  size: number;
  truncated: boolean;
  binary: boolean;
};

export type ChangesListPayload = {
  limit?: number;
};

export type ChangeEntry = {
  path: string;
  old_path?: string;
  status: string;
  index_status?: string;
  worktree_status?: string;
  staged?: boolean;
  unstaged?: boolean;
  untracked?: boolean;
  deleted?: boolean;
  renamed?: boolean;
  restorable?: boolean;
};

export type ChangesListResultPayload = {
  repository: boolean;
  source?: string;
  root?: string;
  entries?: ChangeEntry[];
  count?: number;
  truncated?: boolean;
  clean?: boolean;
  message?: string;
};

export type ChangeDiffPayload = {
  path: string;
};

export type ChangeDiffResultPayload = {
  path: string;
  diff?: string;
  truncated: boolean;
  binary: boolean;
  restorable?: boolean;
  message?: string;
};

export type ChangeRevertPayload = {
  path: string;
};

export type ChangeRevertResultPayload = {
  path: string;
  reverted: boolean;
  message?: string;
};

export type HistoryListPayload = {
  limit?: number;
};

export type HistoryCheckpoint = {
  id: string;
  title?: string;
  reason?: string;
  session_id?: string;
  request_id?: string;
  change_count: number;
  created_at?: string;
};

export type HistoryListResultPayload = {
  root?: string;
  checkpoints?: HistoryCheckpoint[];
  count?: number;
};

export type HistoryDiffPayload = {
  checkpoint_id: string;
};

export type HistoryFileDiff = {
  path: string;
  change_type: string;
  diff?: string;
  truncated: boolean;
  binary: boolean;
  restorable?: boolean;
  message?: string;
};

export type HistoryDiffResultPayload = {
  checkpoint_id: string;
  files?: HistoryFileDiff[];
  count?: number;
  message?: string;
};

export type HistoryRevertPayload = {
  checkpoint_id: string;
};

export type HistoryRevertResultPayload = {
  checkpoint_id: string;
  reverted: boolean;
  paths?: string[];
  message?: string;
};
