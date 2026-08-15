package memory

import (
	"errors"
	"strings"
	"time"
)

type Kind string

const (
	KindExperience Kind = "experience"
	KindFailure    Kind = "failure"
	KindDecision   Kind = "decision"
	KindPreference Kind = "preference"
)

type Status string

const (
	StatusActive     Status = "active"
	StatusArchived   Status = "archived"
	StatusSuperseded Status = "superseded"
	StatusDeleted    Status = "deleted"
)

type ScopeType string

const (
	ScopeGlobal    ScopeType = "global"
	ScopeWorkspace ScopeType = "workspace"
	ScopeProject   ScopeType = "project"
	ScopeSession   ScopeType = "session"
)

type AuthorType string

const (
	AuthorModel  AuthorType = "model"
	AuthorHuman  AuthorType = "human"
	AuthorSystem AuthorType = "system"
)

type SourceType string

const (
	SourceAgentRun SourceType = "agent_run"
	SourceSession  SourceType = "session"
	SourceManual   SourceType = "manual"
	SourceDream    SourceType = "dream"
)

type CandidateStatus string

const (
	CandidatePending  CandidateStatus = "pending"
	CandidateApproved CandidateStatus = "approved"
	CandidateRejected CandidateStatus = "rejected"
	CandidateMerged   CandidateStatus = "merged"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

type DreamStatus string

const (
	DreamPending   DreamStatus = "pending"
	DreamRunning   DreamStatus = "running"
	DreamSucceeded DreamStatus = "succeeded"
	DreamFailed    DreamStatus = "failed"
)

type DreamDecision string

const (
	DecisionCreate      DreamDecision = "create"
	DecisionMerge       DreamDecision = "merge"
	DecisionSupersede   DreamDecision = "supersede"
	DecisionKeepBoth    DreamDecision = "keep_both"
	DecisionReject      DreamDecision = "reject"
	DecisionNeedsReview DreamDecision = "needs_review"
)

type Scope struct {
	Type ScopeType
	Key  string
}

func (scope Scope) Validate() error {
	switch scope.Type {
	case ScopeGlobal:
		if strings.TrimSpace(scope.Key) != "" {
			return errors.New("global memory scope key must be empty")
		}
	case ScopeWorkspace, ScopeProject, ScopeSession:
		if strings.TrimSpace(scope.Key) == "" {
			return errors.New("memory scope key is required")
		}
	default:
		return errors.New("memory scope type is invalid")
	}
	return nil
}

type SourceRef struct {
	Type       SourceType
	SessionID  string
	AgentRunID string
	EventIDs   []string
	CreatedAt  time.Time
}

func (source SourceRef) Validate() error {
	if source.CreatedAt.IsZero() {
		return errors.New("memory source created at is empty")
	}
	switch source.Type {
	case SourceAgentRun:
		if strings.TrimSpace(source.AgentRunID) == "" {
			return errors.New("memory source agent run id is empty")
		}
	case SourceSession:
		if strings.TrimSpace(source.SessionID) == "" {
			return errors.New("memory source session id is empty")
		}
	case SourceManual, SourceDream:
	default:
		return errors.New("memory source type is invalid")
	}
	return nil
}

func validKind(kind Kind) bool {
	switch kind {
	case KindExperience, KindFailure, KindDecision, KindPreference:
		return true
	default:
		return false
	}
}

func validStatus(status Status) bool {
	switch status {
	case StatusActive, StatusArchived, StatusSuperseded, StatusDeleted:
		return true
	default:
		return false
	}
}
