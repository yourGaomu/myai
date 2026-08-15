package memory

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Content struct {
	Goal              string
	ApplicableContext string
	Approach          string
	Result            string
	PainPoints        string
	RootCause         string
	Lessons           string
	Verification      string
}

func (content Content) Validate() error {
	if strings.TrimSpace(content.Goal) == "" {
		return errors.New("memory goal is empty")
	}
	if strings.TrimSpace(content.Approach) == "" && strings.TrimSpace(content.Lessons) == "" {
		return errors.New("memory approach and lessons are both empty")
	}
	return nil
}

type Revision struct {
	ID         string
	Version    int
	Content    Content
	Confidence float64
	Author     AuthorType
	Sources    []SourceRef
	CreatedAt  time.Time
}

func (revision Revision) Validate() error {
	if strings.TrimSpace(revision.ID) == "" {
		return errors.New("memory revision id is empty")
	}
	if revision.Version <= 0 {
		return errors.New("memory revision version must be positive")
	}
	if err := revision.Content.Validate(); err != nil {
		return err
	}
	if math.IsNaN(revision.Confidence) || math.IsInf(revision.Confidence, 0) || revision.Confidence < 0 || revision.Confidence > 1 {
		return errors.New("memory revision confidence must be between 0 and 1")
	}
	switch revision.Author {
	case AuthorModel, AuthorHuman, AuthorSystem:
	default:
		return errors.New("memory revision author is invalid")
	}
	if revision.CreatedAt.IsZero() {
		return errors.New("memory revision created at is empty")
	}
	for _, source := range revision.Sources {
		if err := source.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type Memory struct {
	ID             string
	Title          string
	Kind           Kind
	Scope          Scope
	Tags           []string
	Status         Status
	CurrentVersion int
	Revisions      []Revision
	HumanLocked    bool
	SupersedesID   string
	UseCount       int64
	LastUsedAt     *time.Time
	DeletedAt      *time.Time
	DeletionReason string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (memory Memory) Validate() error {
	if strings.TrimSpace(memory.ID) == "" {
		return errors.New("memory id is empty")
	}
	if strings.TrimSpace(memory.Title) == "" {
		return errors.New("memory title is empty")
	}
	if !validKind(memory.Kind) {
		return errors.New("memory kind is invalid")
	}
	if err := memory.Scope.Validate(); err != nil {
		return err
	}
	if !validStatus(memory.Status) {
		return errors.New("memory status is invalid")
	}
	if memory.CurrentVersion <= 0 || len(memory.Revisions) == 0 {
		return errors.New("memory current revision is missing")
	}
	if memory.UseCount < 0 {
		return errors.New("memory use count is negative")
	}
	if memory.CreatedAt.IsZero() || memory.UpdatedAt.IsZero() {
		return errors.New("memory timestamps are empty")
	}
	seenVersions := make(map[int]struct{}, len(memory.Revisions))
	currentFound := false
	for _, revision := range memory.Revisions {
		if err := revision.Validate(); err != nil {
			return fmt.Errorf("validate memory revision %d: %w", revision.Version, err)
		}
		if _, exists := seenVersions[revision.Version]; exists {
			return fmt.Errorf("memory revision version %d is duplicated", revision.Version)
		}
		seenVersions[revision.Version] = struct{}{}
		if revision.Version == memory.CurrentVersion {
			currentFound = true
		}
	}
	if !currentFound {
		return errors.New("memory current revision does not exist")
	}
	if memory.Status == StatusDeleted && memory.DeletedAt == nil {
		return errors.New("deleted memory timestamp is empty")
	}
	return nil
}

func (memory Memory) CurrentRevision() (Revision, bool) {
	for _, revision := range memory.Revisions {
		if revision.Version == memory.CurrentVersion {
			return revision, true
		}
	}
	return Revision{}, false
}

func (memory *Memory) AppendRevision(revision Revision, now time.Time) error {
	if memory == nil {
		return errors.New("memory is nil")
	}
	revision.Version = memory.CurrentVersion + 1
	if err := revision.Validate(); err != nil {
		return err
	}
	memory.Revisions = append(memory.Revisions, revision)
	memory.CurrentVersion = revision.Version
	memory.UpdatedAt = now
	return memory.Validate()
}

func (memory *Memory) MarkDeleted(now time.Time, reason string) error {
	if memory == nil {
		return errors.New("memory is nil")
	}
	memory.Status = StatusDeleted
	memory.DeletedAt = &now
	memory.DeletionReason = strings.TrimSpace(reason)
	memory.UpdatedAt = now
	return memory.Validate()
}

func (memory *Memory) Restore(now time.Time) error {
	if memory == nil {
		return errors.New("memory is nil")
	}
	memory.Status = StatusActive
	memory.DeletedAt = nil
	memory.DeletionReason = ""
	memory.UpdatedAt = now
	return memory.Validate()
}
