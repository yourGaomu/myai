package memory

import (
	"errors"
	"math"
	"strings"
	"time"
)

type Candidate struct {
	ID             string
	OriginKey      string
	Title          string
	Kind           Kind
	Scope          Scope
	Tags           []string
	Content        Content
	Confidence     float64
	Sources        []SourceRef
	Status         CandidateStatus
	TargetMemoryID string
	ReviewNote     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (candidate Candidate) Validate() error {
	if strings.TrimSpace(candidate.ID) == "" {
		return errors.New("memory candidate id is empty")
	}
	if strings.TrimSpace(candidate.Title) == "" {
		return errors.New("memory candidate title is empty")
	}
	if !validKind(candidate.Kind) {
		return errors.New("memory candidate kind is invalid")
	}
	if err := candidate.Scope.Validate(); err != nil {
		return err
	}
	if err := candidate.Content.Validate(); err != nil {
		return err
	}
	if math.IsNaN(candidate.Confidence) || math.IsInf(candidate.Confidence, 0) || candidate.Confidence < 0 || candidate.Confidence > 1 {
		return errors.New("memory candidate confidence must be between 0 and 1")
	}
	switch candidate.Status {
	case CandidatePending, CandidateApproved, CandidateRejected, CandidateMerged:
	default:
		return errors.New("memory candidate status is invalid")
	}
	if candidate.CreatedAt.IsZero() || candidate.UpdatedAt.IsZero() {
		return errors.New("memory candidate timestamps are empty")
	}
	for _, source := range candidate.Sources {
		if err := source.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type CandidateDraft struct {
	Title      string
	Kind       Kind
	Scope      Scope
	Tags       []string
	Content    Content
	Confidence float64
	Sources    []SourceRef
}
