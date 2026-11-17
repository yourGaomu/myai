package repository

import (
	"errors"
	"time"

	chatmessagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	generationcommand "myai/core/application/chat/generation/command"
	modelport "myai/core/port/model"
	outbox "myai/core/port/outbox"
	repository "myai/core/port/repository"
	"myai/core/session"
)

// PrepareUserMessage maps identities and times once, before acceptance. It does
// not access the database and never serializes a session or a callback.
func (w Writer) PrepareUserMessage(command generationcommand.PersistUserMessage) (outbox.Batch, error) {
	createdAt := command.CreatedAt
	if createdAt.IsZero() {
		createdAt = w.now()
	}
	batch := outbox.Batch{Version: outbox.Version, SessionID: command.SessionID,
		Messages: (chatmessagemapper.Mapper{IDs: w.IDs}).UserTurn(command, createdAt),
		Metadata: messageMetadata(command.SessionSnapshot, command.Model, command.Title, createdAt, false),
	}
	frozen, _, err := outbox.Freeze(batch)
	return frozen, err
}

func (w Writer) PrepareAssistantMessage(current *session.Session, result modelport.ChatResult, createdAt time.Time) (outbox.Batch, error) {
	if current == nil {
		return outbox.Batch{}, errors.New("session is nil")
	}
	if createdAt.IsZero() {
		createdAt = w.now()
	}
	message := (chatmessagemapper.Mapper{IDs: w.IDs}).AssistantMessageFromSession(current, result, createdAt)
	batch := outbox.Batch{Version: outbox.Version, SessionID: current.ID,
		Messages: []repository.MessageRecord{message}, Metadata: messageMetadata(current, current.Model, "", createdAt, true),
	}
	if batch.Metadata.Sequence <= 0 {
		batch.Metadata.Sequence = message.Sequence
	}
	frozen, _, err := outbox.Freeze(batch)
	return frozen, err
}

func messageMetadata(current *session.Session, model, title string, createdAt time.Time, includeUsage bool) *outbox.Metadata {
	m := &outbox.Metadata{Kind: string(session.KindUser), Model: model, Title: title, CreatedAt: createdAt, UpdatedAt: createdAt, IncludeUsage: includeUsage}
	if current == nil {
		return m
	}
	m.Kind = string(session.NormalizeKind(current.Kind))
	m.ParentSessionID, m.ParentTaskID = current.ParentSessionID, current.ParentTaskID
	m.AgentDefinitionID, m.AgentDefinitionVer = current.AgentDefinitionID, current.AgentDefinitionVer
	// Replay never invents the missing security/system state of restricted or
	// child sessions. These must have been synchronously initialized elsewhere.
	m.RequiresExisting = session.NormalizeKind(current.Kind) != session.KindUser || current.ParentSessionID != "" ||
		current.SystemInstruction != "" || len(current.AllowedTools) > 0 || current.EnforceToolAllowlist ||
		current.PermissionCeiling != nil || current.WorkspaceRoot != "" || current.WorkspaceSandboxID != "" ||
		current.MaxToolRounds > 0 || session.NormalizePermissionMode(current.PermissionMode) != session.PermissionModeAsk ||
		session.NormalizeAgentMode(current.AgentMode) == session.AgentModePlan
	if len(current.Messages) > 0 {
		m.Sequence = current.Messages[len(current.Messages)-1].Sequence
	}
	if includeUsage {
		record := (chatmessagemapper.Mapper{}).Session(current, "")
		m.Usage, m.LastUsage = record.Usage, record.LastUsage
	}
	return m
}
