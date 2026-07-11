package sessionapp

import (
	bootstrapcommand "myai/core/application/session/bootstrap/command"
	bootstrapresult "myai/core/application/session/bootstrap/result"
	bootstrapservice "myai/core/application/session/bootstrap/service"
	sessioncommand "myai/core/application/session/command"
	currentresult "myai/core/application/session/current/result"
	currentservice "myai/core/application/session/current/service"
	lifecyclecommand "myai/core/application/session/lifecycle/command"
	lifecycleresult "myai/core/application/session/lifecycle/result"
	lifecycleservice "myai/core/application/session/lifecycle/service"
	loadcommand "myai/core/application/session/load/command"
	loadservice "myai/core/application/session/load/service"
	messagecommand "myai/core/application/session/message/command"
	messageresult "myai/core/application/session/message/result"
	messageservice "myai/core/application/session/message/service"
	persistencecommand "myai/core/application/session/persistence/command"
	persistenceservice "myai/core/application/session/persistence/service"
	querycommand "myai/core/application/session/query/command"
	queryservice "myai/core/application/session/query/service"
	sessionresult "myai/core/application/session/result"
	settingscommand "myai/core/application/session/settings/command"
	settingsservice "myai/core/application/session/settings/service"
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	repository "myai/core/port/repository"
)

type SaveSessionCommand = sessioncommand.SaveSession
type EnsureInMemoryCommand = loadcommand.EnsureInMemory
type LoadService = loadservice.LoadService

type AppendUserMessageCommand = messagecommand.AppendUserMessage
type PrepareRegenerationCommand = messagecommand.PrepareRegeneration
type MessageCommandResult = messageresult.Command
type MessageCommandService = messageservice.CommandService

type BootstrapSessionCommand = bootstrapcommand.Bootstrap
type BootstrapSessionAction = bootstrapresult.Action
type BootstrapSessionResult = bootstrapresult.Bootstrap
type BootstrapSessionService = bootstrapservice.BootstrapService

type CurrentState = currentresult.State
type CurrentSessionService = currentservice.SessionService
type CurrentStateQueryService = currentservice.StateQueryService

type CreateSessionCommand = lifecyclecommand.CreateSession
type LoadSessionCommand = lifecyclecommand.LoadSession
type DeleteSessionCommand = lifecyclecommand.DeleteSession
type RestoreSessionCommand = lifecyclecommand.RestoreSession
type ClearSessionCommand = lifecyclecommand.ClearSession
type LifecycleResult = lifecycleresult.Lifecycle
type DeleteSessionResult = lifecycleresult.DeleteSession
type LifecycleService = lifecycleservice.LifecycleService
type LifecycleUseCase = lifecycleservice.UseCase

type ListAssetsCommand = querycommand.ListAssets
type AssetListItem = sessionresult.AssetListItem
type MessageListItem = sessionresult.MessageListItem
type MessageHistoryMetaResult = sessionresult.MessageHistoryMeta
type SessionListItem = sessionresult.SessionListItem
type TokenUsageResult = sessionresult.TokenUsage
type SessionQueryService = queryservice.SessionQueryService
type MessageQueryService = queryservice.MessageQueryService

type BuildSessionRecordCommand = persistencecommand.BuildRecord
type PrepareSessionRecordCommand = persistencecommand.PrepareRecord
type SessionPersistenceService = persistenceservice.PersistenceService

type SwitchModelCommand = settingscommand.SwitchModel
type SetPermissionModeCommand = settingscommand.SetPermissionMode
type SetAgentModeCommand = settingscommand.SetAgentMode
type SetContextWindowCommand = settingscommand.SetContextWindow
type SettingsService = settingsservice.SettingsService
type SettingsUseCase = settingsservice.UseCase

const (
	BootstrapSessionLoaded  = bootstrapresult.ActionLoaded
	BootstrapSessionCreated = bootstrapresult.ActionCreated
	BootstrapSessionReused  = bootstrapresult.ActionReused
	SessionActionNew        = "new"
	SessionActionLoad       = "load"
	SessionActionDelete     = "delete"
	SessionActionRestore    = "restore"
	SessionActionClear      = "clear"
	DefaultTitle            = persistenceservice.DefaultTitle
)

func TokenUsageRecord(usage llm.TokenUsage) *repository.TokenUsageRecord {
	return persistenceservice.TokenUsageRecord(usage)
}

func TokenUsageFromRecord(record *repository.TokenUsageRecord) llm.TokenUsage {
	return loadservice.TokenUsageFromRecord(record)
}

func MessageHistoryMetaFromRecords(sessionID string, records []repository.MessageRecord) repository.MessageHistoryMeta {
	return queryservice.MessageHistoryMetaFromRecords(sessionID, records)
}

func MessagesAfterID(records []repository.MessageRecord, afterMessageID string, limit int) ([]repository.MessageRecord, bool, error) {
	return queryservice.MessagesAfterID(records, afterMessageID, limit)
}

func MessagesFromRecords(records []repository.MessageRecord) []domainmessage.Message {
	return loadservice.MessagesFromRecords(records)
}

func BuildSessionRecord(command BuildSessionRecordCommand) repository.SessionRecord {
	return persistenceservice.BuildSessionRecord(command)
}

func PrepareSessionRecordForSave(command PrepareSessionRecordCommand) (repository.SessionRecord, error) {
	return persistenceservice.PrepareSessionRecordForSave(command)
}

func SessionListItems(records []repository.SessionRecord) []SessionListItem {
	return queryservice.SessionListItems(records)
}

func MessageListItems(records []repository.MessageRecord) []MessageListItem {
	return queryservice.MessageListItems(records)
}

func AssetListItems(records []repository.AssetRecord) []AssetListItem {
	return queryservice.AssetListItems(records)
}

func MessageHistoryMetaResultFromRecord(record repository.MessageHistoryMeta) MessageHistoryMetaResult {
	return queryservice.MessageHistoryMetaResultFromRecord(record)
}

func TokenUsageResultFromRecord(record *repository.TokenUsageRecord) *TokenUsageResult {
	return queryservice.TokenUsageResultFromRecord(record)
}
