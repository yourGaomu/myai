package chat

import (
	compactioncommand "myai/core/application/chat/compaction/command"
	compactionresult "myai/core/application/chat/compaction/result"
	compactionservice "myai/core/application/chat/compaction/service"
	chatcontextservice "myai/core/application/chat/context/service"
	generationapi "myai/core/application/chat/generation/api"
	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	generationresult "myai/core/application/chat/generation/result"
	generationservice "myai/core/application/chat/generation/service"
	planapi "myai/core/application/chat/plan/api"
	plancommand "myai/core/application/chat/plan/command"
	planport "myai/core/application/chat/plan/port"
	planresult "myai/core/application/chat/plan/result"
	planservice "myai/core/application/chat/plan/service"
)

type ContextSnapshotService = chatcontextservice.SnapshotService
type ContextQueryService = chatcontextservice.QueryService
type CompactInfo = compactionresult.CompactInfo
type CompactSessionCommand = compactioncommand.CompactSession
type CompactService = compactionservice.CompactService
type SessionCompactionService = compactionservice.SessionService
type SummaryService = compactionservice.SummaryService
type CommitCommand = generationcommand.Commit
type CommitResult = generationresult.Commit
type ResponseCommitService = generationservice.ResponseCommitService
type PersistUserMessageCommand = generationcommand.PersistUserMessage
type ToolExecutionCommand = generationcommand.ToolExecution
type ToolExecutionRecordCommand = generationcommand.ToolExecutionRecord
type ToolExecutionResult = generationresult.ToolExecution
type RunCommand = generationcommand.Run
type AgentLoopService = generationservice.AgentLoopService
type AssistantGenerationCommand = generationcommand.AssistantGeneration
type GenerationResponse = generationresult.GenerationResponse
type AssistantGenerationService = generationservice.AssistantGenerationService
type GenerationTaskCommand = generationcommand.GenerationTask
type TaskRecord = generationcommand.TaskRecord
type GenerationTaskService = generationservice.TaskService
type RequestIDGenerator = generationport.RequestIDGenerator
type GenerationTaskRecorderFactory = generationport.TaskRecorderFactory
type GenerationTaskRecorder = generationport.TaskRecorder
type GenerationTaskHandler = generationapi.Generator
type PlanExecutionCommand = plancommand.Execute
type PlanExecutionResult = planresult.Execution
type PlanUpdateSink = planport.UpdateSink
type PlanExecutionService = planservice.ExecutionService
type PlanExecutionServiceAPI = planapi.Service

const DefaultMaxToolRounds = generationservice.DefaultMaxToolRounds

const DefaultCompactKeepChunks = compactionservice.DefaultKeepChunks

var ErrNotEnoughHistoryToCompact = compactionservice.ErrNotEnoughHistory
