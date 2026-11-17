package memory

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
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

// ContentUpdatedAt uses revision timestamps so usage updates made by older
// releases cannot make old content appear new. CreatedAt covers legacy records
// without a usable revision timestamp; no stored fields need to be migrated.
func (memory Memory) ContentUpdatedAt() time.Time {
	latest := time.Time{}
	for _, revision := range memory.Revisions {
		if revision.CreatedAt.After(latest) {
			latest = revision.CreatedAt
		}
	}
	if latest.IsZero() {
		return memory.CreatedAt
	}
	return latest
}

func (memory Memory) SearchText() string {
	revision, ok := memory.CurrentRevision()
	if !ok {
		return ""
	}
	content := revision.Content
	return strings.Join([]string{
		memory.Title, strings.Join(memory.Tags, "\n"), content.Goal, content.ApplicableContext,
		content.Approach, content.Result, content.PainPoints, content.RootCause, content.Lessons, content.Verification,
	}, "\n")
}

// SearchTerms keeps words intact outside Han text. Han runs also contribute
// adjacent pairs so Chinese queries can match phrases without a tokenizer.
func SearchTerms(text string) []string {
	terms := make(map[string]struct{})
	var run []rune
	flush := func() {
		if len(run) >= 2 {
			terms[string(run)] = struct{}{}
			if unicode.Is(unicode.Han, run[0]) {
				for index := 0; index+1 < len(run); index++ {
					terms[string(run[index:index+2])] = struct{}{}
				}
			}
		}
		run = run[:0]
	}
	for _, character := range strings.ToLower(text) {
		if !unicode.IsLetter(character) && !unicode.IsNumber(character) {
			flush()
			continue
		}
		if len(run) > 0 && unicode.Is(unicode.Han, run[0]) != unicode.Is(unicode.Han, character) {
			flush()
		}
		run = append(run, character)
	}
	flush()
	result := make([]string, 0, len(terms))
	for term := range terms {
		result = append(result, term)
	}
	sort.Strings(result)
	return result
}

// SearchTermPattern is usable by both Go and Mongo regular expressions. Han
// pairs may occur inside sentences; other words require Unicode boundaries.
func SearchTermPattern(term string) string {
	literal := regexp.QuoteMeta(term)
	for _, character := range term {
		if unicode.Is(unicode.Han, character) {
			return literal
		}
	}
	return `(^|[^\p{L}\p{N}]|\p{Han})` + literal + `($|[^\p{L}\p{N}]|\p{Han})`
}

func (memory Memory) MatchesSearchTerms(terms []string) bool {
	available := make(map[string]struct{})
	for _, term := range SearchTerms(memory.SearchText()) {
		available[term] = struct{}{}
	}
	for _, term := range terms {
		if _, ok := available[term]; ok {
			return true
		}
	}
	return false
}

// ContainsSearchPhrase applies the same word boundaries to title and tag
// bonuses. Empty labels and substrings within unrelated words earn no bonus.
func ContainsSearchPhrase(text string, phrase string) bool {
	phrase = strings.ToLower(strings.TrimSpace(phrase))
	if phrase == "" || len(SearchTerms(phrase)) == 0 {
		return false
	}
	text = strings.ToLower(text)
	runes := []rune(phrase)
	pattern := regexp.QuoteMeta(phrase)
	if wordCharacter(runes[0]) {
		pattern = `(^|[^\p{L}\p{N}]|\p{Han})` + pattern
	}
	if wordCharacter(runes[len(runes)-1]) {
		pattern += `($|[^\p{L}\p{N}]|\p{Han})`
	}
	return regexp.MustCompile(pattern).MatchString(text)
}

func wordCharacter(character rune) bool {
	return (unicode.IsLetter(character) || unicode.IsNumber(character)) && !unicode.Is(unicode.Han, character)
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
