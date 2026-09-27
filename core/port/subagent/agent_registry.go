package subagent

import "errors"

var ErrAgentPathTaken = errors.New("agent path is already reserved")

// AgentPathRegistry reserves logical paths before a child is persisted. The
// reservation is intentionally separate from task persistence so admission is
// atomic within one process and can later be backed by a durable AgentRegistry.
type AgentPathRegistry interface {
	Reserve(path string) error
	Release(path string)
}
