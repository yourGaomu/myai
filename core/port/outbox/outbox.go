package outbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	repository "myai/core/port/repository"
)

const Version = 1

var (
	ErrConflict          = errors.New("outbox identity payload conflict")
	ErrCapacity          = errors.New("outbox capacity exceeded")
	ErrBlocked           = errors.New("session persistence is blocked")
	ErrInvalidRecord     = errors.New("outbox contains quarantined records")
	ErrClosed            = errors.New("outbox journal is closed")
	ErrClosureNotDurable = errors.New("durable session queue only accepts typed records")
)

// Metadata is deliberately a whitelist. It cannot carry a whole session, plan,
// model credentials, environment, or runtime configuration. Usage is cumulative.
// Defaults for a missing session are conservative; existing settings are owned
// by the session persistence service, not by replay.
type Metadata struct {
	Kind               string
	ParentSessionID    string
	ParentTaskID       string
	AgentDefinitionID  string
	AgentDefinitionVer int64
	Model              string
	Title              string
	Usage              *repository.TokenUsageRecord
	LastUsage          *repository.TokenUsageRecord
	Sequence           int64
	IncludeUsage       bool
	// Restricted and child sessions must already exist with their security and
	// system settings. Replay cannot reconstruct those settings from a message.
	RequiresExisting bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (m Metadata) SessionRecord(sessionID string) repository.SessionRecord {
	kind := m.Kind
	if kind == "" {
		kind = "user"
	}
	title := m.Title
	if title == "" {
		title = "New chat"
	}
	return repository.SessionRecord{
		ID: sessionID, Kind: kind, ParentSessionID: m.ParentSessionID,
		ParentTaskID: m.ParentTaskID, AgentDefinitionID: m.AgentDefinitionID,
		AgentDefinitionVer: m.AgentDefinitionVer, Model: m.Model, Title: title,
		AgentMode: "chat", PermissionMode: "ask", Usage: m.Usage,
		LastUsage: m.LastUsage, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// Batch contains already mapped, immutable operations only. Replacement of a
// transcript is a synchronous transaction and is never an outbox operation.
type Batch struct {
	Version   int
	ID        string
	SessionID string
	Messages  []repository.MessageRecord
	Assets    []repository.AssetRecord
	Metadata  *Metadata
}

func Hash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (b Batch) Validate() error {
	if b.Version != Version {
		return fmt.Errorf("unsupported outbox version %d", b.Version)
	}
	if strings.TrimSpace(b.SessionID) == "" {
		return errors.New("outbox session id is required")
	}
	if len(b.Messages) == 0 && len(b.Assets) == 0 && b.Metadata == nil {
		return errors.New("outbox batch is empty")
	}
	identities := make(map[string]string)
	check := func(kind, id, sessionID string, value any) error {
		if strings.TrimSpace(id) == "" || sessionID != b.SessionID {
			return fmt.Errorf("invalid outbox %s identity or session", kind)
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		key, hash := kind+":"+id, Hash(data)
		if prior, ok := identities[key]; ok && prior != hash {
			return ErrConflict
		}
		identities[key] = hash
		return nil
	}
	for _, record := range b.Messages {
		switch record.Role {
		case repository.RoleSystem, repository.RoleUser, repository.RoleAssistant, repository.RoleToolCall, repository.RoleTool:
		default:
			return errors.New("invalid outbox message role")
		}
		if record.CreatedAt.IsZero() || record.Sequence <= 0 {
			return errors.New("outbox message requires stable timestamp and sequence")
		}
		if err := check("message", record.ID, record.SessionID, record); err != nil {
			return err
		}
	}
	for _, record := range b.Assets {
		if record.CreatedAt.IsZero() || record.Deleted || record.DeletedAt != nil {
			return errors.New("outbox asset must be an append with a stable timestamp")
		}
		if err := check("asset", record.ID, record.SessionID, record); err != nil {
			return err
		}
	}
	if m := b.Metadata; m != nil {
		if m.CreatedAt.IsZero() || m.UpdatedAt.IsZero() {
			return errors.New("outbox metadata requires stable timestamps")
		}
		if m.IncludeUsage && (m.Usage != nil || m.LastUsage != nil) && m.Sequence <= 0 {
			return errors.New("outbox usage requires a positive sequence")
		}
	}
	return nil
}

type Entry struct {
	Sequence int64
	Batch    Batch
	State    string
}

const (
	Pending   = "pending"
	Done      = "done"
	Cancelled = "cancelled"
)

type Stats struct {
	Pending      int
	Done         int
	Cancelled    int
	Invalid      int
	Blocked      int
	Bytes        int64
	PayloadBytes int64
}

// SafeError preserves errors.Is without exposing backend text to remote callers.
// Backend errors may include record bodies, connection URIs or filesystem paths.
type safeError struct {
	operation string
	cause     error
}

func (e safeError) Error() string        { return e.operation + " failed" }
func (e safeError) Is(target error) bool { return errors.Is(e.cause, target) }
func (e safeError) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		fmt.Fprintf(state, "%q", e.Error())
		return
	}
	fmt.Fprint(state, e.Error())
}
func SafeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return safeError{operation: operation, cause: err}
}
