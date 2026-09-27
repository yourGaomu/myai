package subagent

import (
	"errors"

	domainsubagent "myai/core/domain/subagent"
)

var (
	ErrAgentRuntimeNotFound = errors.New("subagent runtime is not registered")
	ErrAgentTurnActive      = errors.New("subagent already has an active turn")
	ErrAgentTurnMismatch    = errors.New("subagent turn does not own the runtime")
	ErrAgentRuntimeLoaded   = errors.New("subagent runtime is already loaded")
	ErrAgentRuntimeUnloaded = errors.New("subagent runtime is already unloaded")
)

type AgentRuntimeSnapshot = domainsubagent.AgentRuntimeSnapshot

// AgentRuntimeManager owns process-local runtime state. Persistence remains
// the source of truth for history; this port only controls loaded residency
// and the single active turn admission invariant.
type AgentRuntimeManager interface {
	Register(thread domainsubagent.AgentThread) error
	Remove(threadID string) error
	ReserveTurn(threadID, turnID string) error
	ReleaseTurn(threadID, turnID string) error
	SetStatus(threadID string, status domainsubagent.AgentStatus) error
	MarkLoaded(threadID string) error
	MarkUnloaded(threadID string) error
	Inspect(threadID string) (AgentRuntimeSnapshot, error)
	List() []AgentRuntimeSnapshot
}
