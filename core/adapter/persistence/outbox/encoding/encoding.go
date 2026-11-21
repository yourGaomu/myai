package encoding

import (
	"encoding/json"
	"time"

	outbox "myai/core/port/outbox"
	"myai/core/port/repository"
)

// Keep the v1 field names and order unchanged when the usage flags are false:
// both the whole batch and individual message bytes participate in deduplication.
// The shallow fields override the corresponding promoted fields for JSON.
type messageRecord = repository.MessageRecord
type messageDocument struct {
	messageRecord
	PromptCacheReported   bool `json:",omitempty"`
	PromptCacheIncomplete bool `json:",omitempty"`
}

func Message(record repository.MessageRecord) messageDocument {
	return messageDocument{record, record.PromptCacheReported, record.PromptCacheIncomplete}
}

type tokenUsageRecord = repository.TokenUsageRecord
type tokenUsageDocument struct {
	tokenUsageRecord
	PromptCacheReported   bool `json:",omitempty"`
	PromptCacheIncomplete bool `json:",omitempty"`
}

func usageDocument(record *repository.TokenUsageRecord) *tokenUsageDocument {
	if record == nil {
		return nil
	}
	return &tokenUsageDocument{*record, record.PromptCacheReported, record.PromptCacheIncomplete}
}

type metadataDocument struct {
	Kind               string
	ParentSessionID    string
	ParentTaskID       string
	AgentDefinitionID  string
	AgentDefinitionVer int64
	Model              string
	Title              string
	Usage              *tokenUsageDocument
	LastUsage          *tokenUsageDocument
	Sequence           int64
	IncludeUsage       bool
	RequiresExisting   bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type batchDocument struct {
	Version   int
	ID        string
	SessionID string
	Messages  []messageDocument
	Assets    []repository.AssetRecord
	Metadata  *metadataDocument
}

// Freeze supplies the envelope identity and makes an independent JSON snapshot.
// A retry must reuse this batch; mapping again may generate different record IDs.
func Freeze(batch outbox.Batch) (outbox.Batch, []byte, error) {
	if batch.Version == 0 {
		batch.Version = outbox.Version
	}
	if err := batch.Validate(); err != nil {
		return outbox.Batch{}, nil, err
	}
	document := batchDocument{Version: batch.Version, ID: batch.ID, SessionID: batch.SessionID, Assets: batch.Assets}
	if batch.Messages != nil {
		document.Messages = make([]messageDocument, len(batch.Messages))
		for i, record := range batch.Messages {
			document.Messages[i] = Message(record)
		}
	}
	if m := batch.Metadata; m != nil {
		document.Metadata = &metadataDocument{
			Kind: m.Kind, ParentSessionID: m.ParentSessionID, ParentTaskID: m.ParentTaskID,
			AgentDefinitionID: m.AgentDefinitionID, AgentDefinitionVer: m.AgentDefinitionVer, Model: m.Model, Title: m.Title,
			Usage: usageDocument(m.Usage), LastUsage: usageDocument(m.LastUsage), Sequence: m.Sequence, IncludeUsage: m.IncludeUsage,
			RequiresExisting: m.RequiresExisting, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		}
	}
	if document.ID == "" {
		encoded, err := json.Marshal(document)
		if err != nil {
			return outbox.Batch{}, nil, err
		}
		document.ID = outbox.Hash(encoded)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return outbox.Batch{}, nil, err
	}
	var frozen outbox.Batch
	if err := json.Unmarshal(encoded, &frozen); err != nil {
		return outbox.Batch{}, nil, err
	}
	return frozen, encoded, nil
}
