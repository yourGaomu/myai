package subagent

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	domainworkspace "myai/core/domain/workspace"
)

const (
	MaxDefinitionNameRunes        = 100
	MaxDefinitionDescriptionRunes = 1000
	MaxSystemPromptRunes          = 12000
	MaxAllowedTools               = 128
	DefaultMaxTurns               = 12
	DefaultTimeoutSeconds         = 300
	MaxTimeoutSeconds             = 3600
)

type DefinitionSource string

const (
	DefinitionSourceBuiltin DefinitionSource = "builtin"
	DefinitionSourceUser    DefinitionSource = "user"
)

type CapabilityMode string

const (
	CapabilityModeReadOnly  CapabilityMode = "read_only"
	CapabilityModeReadWrite CapabilityMode = "read_write"
	CapabilityModeExecute   CapabilityMode = "execute"
	CapabilityModeAll       CapabilityMode = "all"
)

type Definition struct {
	ID             string
	Name           string
	Description    string
	SystemPrompt   string
	ModelID        string
	AllowedTools   []string
	CapabilityMode CapabilityMode
	IsolationMode  domainworkspace.IsolationMode
	MaxTurns       int
	TimeoutSeconds int
	Enabled        bool
	Version        int64
	Source         DefinitionSource
	Deleted        bool
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DefinitionSnapshot struct {
	ID             string
	Name           string
	SystemPrompt   string
	ModelID        string
	AllowedTools   []string
	CapabilityMode CapabilityMode
	IsolationMode  domainworkspace.IsolationMode
	MaxTurns       int
	TimeoutSeconds int
	Version        int64
}

func (definition Definition) Validate() error {
	if strings.TrimSpace(definition.ID) == "" {
		return errors.New("subagent definition id is required")
	}
	if strings.TrimSpace(definition.Name) == "" {
		return errors.New("subagent definition name is required")
	}
	if utf8.RuneCountInString(definition.Name) > MaxDefinitionNameRunes {
		return fmt.Errorf("subagent definition name must not exceed %d characters", MaxDefinitionNameRunes)
	}
	if utf8.RuneCountInString(definition.Description) > MaxDefinitionDescriptionRunes {
		return fmt.Errorf("subagent definition description must not exceed %d characters", MaxDefinitionDescriptionRunes)
	}
	if strings.TrimSpace(definition.SystemPrompt) == "" {
		return errors.New("subagent system prompt is required")
	}
	if utf8.RuneCountInString(definition.SystemPrompt) > MaxSystemPromptRunes {
		return fmt.Errorf("subagent system prompt must not exceed %d characters", MaxSystemPromptRunes)
	}
	if len(definition.AllowedTools) > MaxAllowedTools {
		return fmt.Errorf("subagent definition must not contain more than %d tools", MaxAllowedTools)
	}
	if err := validateCapabilityMode(definition.CapabilityMode); err != nil {
		return err
	}
	if err := domainworkspace.ValidateIsolationMode(definition.IsolationMode); err != nil {
		return err
	}
	maxTurns := definition.MaxTurns
	if maxTurns == 0 {
		maxTurns = DefaultMaxTurns
	}
	if maxTurns < 1 || maxTurns > 64 {
		return errors.New("subagent max turns must be between 1 and 64")
	}
	timeout := definition.TimeoutSeconds
	if timeout == 0 {
		timeout = DefaultTimeoutSeconds
	}
	if timeout < 1 || timeout > MaxTimeoutSeconds {
		return fmt.Errorf("subagent timeout must be between 1 and %d seconds", MaxTimeoutSeconds)
	}
	return validateToolNames(definition.AllowedTools)
}

func (definition Definition) Normalized() Definition {
	definition.ID = strings.TrimSpace(definition.ID)
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Description = strings.TrimSpace(definition.Description)
	definition.SystemPrompt = strings.TrimSpace(definition.SystemPrompt)
	definition.ModelID = strings.TrimSpace(definition.ModelID)
	definition.AllowedTools = normalizeToolNames(definition.AllowedTools)
	if definition.CapabilityMode == "" {
		definition.CapabilityMode = CapabilityModeReadOnly
	}
	definition.IsolationMode = domainworkspace.NormalizeIsolationMode(definition.IsolationMode)
	if definition.MaxTurns == 0 {
		definition.MaxTurns = DefaultMaxTurns
	}
	if definition.TimeoutSeconds == 0 {
		definition.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if definition.Version < 1 {
		definition.Version = 1
	}
	if definition.Source == "" {
		definition.Source = DefinitionSourceUser
	}
	return definition
}

func (definition Definition) Snapshot() DefinitionSnapshot {
	definition = definition.Normalized()
	return DefinitionSnapshot{
		ID: definition.ID, Name: definition.Name, SystemPrompt: definition.SystemPrompt,
		ModelID: definition.ModelID, AllowedTools: cloneStrings(definition.AllowedTools),
		CapabilityMode: definition.CapabilityMode, IsolationMode: definition.IsolationMode,
		MaxTurns: definition.MaxTurns, TimeoutSeconds: definition.TimeoutSeconds, Version: definition.Version,
	}
}

func CloneDefinition(definition Definition) Definition {
	definition.AllowedTools = cloneStrings(definition.AllowedTools)
	definition.DeletedAt = cloneTime(definition.DeletedAt)
	return definition
}

func CloneDefinitionSnapshot(snapshot DefinitionSnapshot) DefinitionSnapshot {
	snapshot.AllowedTools = cloneStrings(snapshot.AllowedTools)
	return snapshot
}

func validateCapabilityMode(mode CapabilityMode) error {
	if mode == "" {
		return nil
	}
	switch mode {
	case CapabilityModeReadOnly, CapabilityModeReadWrite, CapabilityModeExecute, CapabilityModeAll:
		return nil
	default:
		return fmt.Errorf("unsupported subagent capability mode %q", mode)
	}
}

func validateToolNames(names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("subagent tool name is required")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate subagent tool %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func normalizeToolNames(names []string) []string {
	if names == nil {
		return nil
	}
	result := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func cloneStrings(source []string) []string {
	if source == nil {
		return nil
	}
	return append([]string(nil), source...)
}

func cloneTime(source *time.Time) *time.Time {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}
