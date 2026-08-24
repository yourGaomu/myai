package compaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const CurrentVersion = 1

// Summary stores the durable sections emitted by the compaction prompt.
// Checkpoint.Summary keeps the canonical JSON source; old Markdown summaries
// remain readable through the legacy compatibility path.
type Summary struct {
	CurrentGoal      string
	Preferences      []string
	Constraints      []string
	Decisions        []string
	CompletedWork    []string
	ModifiedFiles    []string
	ToolVerification []string
	Problems         []string
	OpenTasks        []string
	NextSteps        []string
	References       []string
}

var summaryJSONFields = []string{
	"current_goal", "preferences", "constraints", "decisions", "completed_work",
	"modified_files", "tool_verification", "problems", "open_tasks", "next_steps", "references",
}

// Checkpoint describes exactly which message prefix a summary replaces.
// SourceEndMessage is an exclusive message index in Session.Messages. The
// current implementation always starts at zero because a summary represents
// the complete compacted prefix, including any previous checkpoint.
type Checkpoint struct {
	Version            int
	SourceStartMessage int
	SourceEndMessage   int
	SourceHistoryHash  string
	Summary            string
	SummaryData        Summary
	CreatedAt          time.Time
}

func NewCheckpoint(summary string, sourceStartMessage, sourceEndMessage int, sourceHistoryHash string, createdAt time.Time) (Checkpoint, error) {
	checkpoint := Checkpoint{
		Version:            CurrentVersion,
		SourceStartMessage: sourceStartMessage,
		SourceEndMessage:   sourceEndMessage,
		SourceHistoryHash:  strings.TrimSpace(sourceHistoryHash),
		Summary:            strings.TrimSpace(summary),
		CreatedAt:          createdAt,
	}
	parsed, err := DecodeJSON(checkpoint.Summary)
	if err != nil {
		return Checkpoint{}, err
	}
	checkpoint.SummaryData = parsed
	if err := checkpoint.Validate(); err != nil {
		return Checkpoint{}, err
	}
	return checkpoint, nil
}

// LegacyCheckpoint wraps a pre-checkpoint summary without pretending that it
// has a verifiable source range. It is used only for old persisted sessions.
func LegacyCheckpoint(summary string, compactedMessages int, sourceHistoryHash string) Checkpoint {
	checkpoint := Checkpoint{
		Version:            0,
		SourceStartMessage: 0,
		SourceEndMessage:   compactedMessages,
		SourceHistoryHash:  strings.TrimSpace(sourceHistoryHash),
		Summary:            strings.TrimSpace(summary),
		CreatedAt:          time.Time{},
	}
	checkpoint.SummaryData = ParseSummaryOrJSON(checkpoint.Summary)
	return checkpoint
}

// DecodeJSON validates the exact object shape emitted by the summarizer. It
// intentionally rejects Markdown, code fences, unknown fields and trailing
// values so malformed checkpoints never enter the persisted session state.
func DecodeJSON(raw string) (Summary, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Summary{}, errors.New("summary JSON is empty")
	}

	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(&fields); err != nil {
		return Summary{}, fmt.Errorf("decode summary JSON: %w", err)
	}
	if fields == nil {
		return Summary{}, errors.New("summary JSON must be an object")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Summary{}, err
	}
	if len(fields) != len(summaryJSONFields) {
		return Summary{}, fmt.Errorf("summary JSON must contain exactly %d fields", len(summaryJSONFields))
	}
	for _, field := range summaryJSONFields {
		if _, ok := fields[field]; !ok {
			return Summary{}, fmt.Errorf("summary JSON field %q is missing", field)
		}
	}

	var summary Summary
	for _, field := range summaryJSONFields {
		if err := decodeSummaryField(fields[field], field, &summary); err != nil {
			return Summary{}, err
		}
	}
	return normalizeSummary(summary), nil
}

func decodeSummaryField(raw json.RawMessage, field string, summary *Summary) error {
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("summary JSON field %q has invalid type", field)
	}
	var target any
	switch field {
	case "current_goal":
		target = &summary.CurrentGoal
	case "preferences":
		target = &summary.Preferences
	case "constraints":
		target = &summary.Constraints
	case "decisions":
		target = &summary.Decisions
	case "completed_work":
		target = &summary.CompletedWork
	case "modified_files":
		target = &summary.ModifiedFiles
	case "tool_verification":
		target = &summary.ToolVerification
	case "problems":
		target = &summary.Problems
	case "open_tasks":
		target = &summary.OpenTasks
	case "next_steps":
		target = &summary.NextSteps
	case "references":
		target = &summary.References
	default:
		return fmt.Errorf("summary JSON field %q is unsupported", field)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode summary JSON field %q: %w", field, err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("summary JSON contains trailing data")
		}
		return fmt.Errorf("read summary JSON trailing data: %w", err)
	}
	return nil
}

func EncodeJSON(summary Summary) (string, error) {
	summary = normalizeSummary(summary)
	encoded, err := json.Marshal(map[string]any{
		"current_goal":      summary.CurrentGoal,
		"preferences":       summary.Preferences,
		"constraints":       summary.Constraints,
		"decisions":         summary.Decisions,
		"completed_work":    summary.CompletedWork,
		"modified_files":    summary.ModifiedFiles,
		"tool_verification": summary.ToolVerification,
		"problems":          summary.Problems,
		"open_tasks":        summary.OpenTasks,
		"next_steps":        summary.NextSteps,
		"references":        summary.References,
	})
	if err != nil {
		return "", fmt.Errorf("encode summary JSON: %w", err)
	}
	return string(encoded), nil
}

func ParseSummaryOrJSON(raw string) Summary {
	if summary, err := DecodeJSON(raw); err == nil {
		return summary
	}
	return ParseSummary(raw)
}

func normalizeSummary(summary Summary) Summary {
	if summary.Preferences == nil {
		summary.Preferences = []string{}
	}
	if summary.Constraints == nil {
		summary.Constraints = []string{}
	}
	if summary.Decisions == nil {
		summary.Decisions = []string{}
	}
	if summary.CompletedWork == nil {
		summary.CompletedWork = []string{}
	}
	if summary.ModifiedFiles == nil {
		summary.ModifiedFiles = []string{}
	}
	if summary.ToolVerification == nil {
		summary.ToolVerification = []string{}
	}
	if summary.Problems == nil {
		summary.Problems = []string{}
	}
	if summary.OpenTasks == nil {
		summary.OpenTasks = []string{}
	}
	if summary.NextSteps == nil {
		summary.NextSteps = []string{}
	}
	if summary.References == nil {
		summary.References = []string{}
	}
	return summary
}

func (summary Summary) HasContent() bool {
	return strings.TrimSpace(summary.CurrentGoal) != "" || len(summary.Preferences) > 0 || len(summary.Constraints) > 0 ||
		len(summary.Decisions) > 0 || len(summary.CompletedWork) > 0 || len(summary.ModifiedFiles) > 0 ||
		len(summary.ToolVerification) > 0 || len(summary.Problems) > 0 || len(summary.OpenTasks) > 0 ||
		len(summary.NextSteps) > 0 || len(summary.References) > 0
}

// RenderMarkdown is only for human-facing context inspection. The model and
// context window continue to use Checkpoint.Summary, which remains canonical
// JSON for new checkpoints.
func (summary Summary) RenderMarkdown() string {
	var builder strings.Builder
	writeSection := func(title string, scalar string, values []string) {
		builder.WriteString("## ")
		builder.WriteString(title)
		builder.WriteByte('\n')
		if strings.TrimSpace(scalar) != "" {
			builder.WriteString(strings.TrimSpace(scalar))
			builder.WriteByte('\n')
		}
		if len(values) == 0 && strings.TrimSpace(scalar) == "" {
			builder.WriteString("- 无明确记录\n")
			return
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				builder.WriteString("- ")
				builder.WriteString(value)
				builder.WriteByte('\n')
			}
		}
	}
	writeSection("当前目标", summary.CurrentGoal, nil)
	writeSection("用户偏好与约束", "", append(append([]string{}, summary.Preferences...), summary.Constraints...))
	writeSection("关键决策", "", summary.Decisions)
	writeSection("已完成工作", "", summary.CompletedWork)
	writeSection("修改文件", "", summary.ModifiedFiles)
	writeSection("工具与验证", "", summary.ToolVerification)
	writeSection("问题与根因", "", summary.Problems)
	writeSection("未完成任务与下一步", "", append(append([]string{}, summary.OpenTasks...), summary.NextSteps...))
	writeSection("重要引用", "", summary.References)
	return strings.TrimSpace(builder.String())
}

func (checkpoint Checkpoint) DisplaySummary() string {
	if checkpoint.SummaryData.HasContent() {
		return checkpoint.SummaryData.RenderMarkdown()
	}
	return checkpoint.Summary
}

func (checkpoint Checkpoint) Validate() error {
	if strings.TrimSpace(checkpoint.Summary) == "" {
		return errors.New("compaction checkpoint summary is empty")
	}
	if checkpoint.Version < 0 || checkpoint.Version > CurrentVersion {
		return errors.New("unsupported compaction checkpoint version")
	}
	if checkpoint.SourceStartMessage < 0 || checkpoint.SourceEndMessage <= checkpoint.SourceStartMessage {
		return errors.New("invalid compaction checkpoint source range")
	}
	if checkpoint.Version >= CurrentVersion && strings.TrimSpace(checkpoint.SourceHistoryHash) == "" {
		return errors.New("compaction checkpoint source history hash is empty")
	}
	if checkpoint.Version >= CurrentVersion {
		if _, err := DecodeJSON(checkpoint.Summary); err != nil {
			return fmt.Errorf("invalid structured compaction summary: %w", err)
		}
	}
	return nil
}

func (checkpoint Checkpoint) Clone() *Checkpoint {
	cloned := checkpoint
	cloned.SummaryData = cloneSummary(checkpoint.SummaryData)
	return &cloned
}

func (summary Summary) Clone() Summary {
	return cloneSummary(summary)
}

func cloneSummary(summary Summary) Summary {
	cloned := summary
	cloned.Preferences = append([]string(nil), summary.Preferences...)
	cloned.Constraints = append([]string(nil), summary.Constraints...)
	cloned.Decisions = append([]string(nil), summary.Decisions...)
	cloned.CompletedWork = append([]string(nil), summary.CompletedWork...)
	cloned.ModifiedFiles = append([]string(nil), summary.ModifiedFiles...)
	cloned.ToolVerification = append([]string(nil), summary.ToolVerification...)
	cloned.Problems = append([]string(nil), summary.Problems...)
	cloned.OpenTasks = append([]string(nil), summary.OpenTasks...)
	cloned.NextSteps = append([]string(nil), summary.NextSteps...)
	cloned.References = append([]string(nil), summary.References...)
	return cloned
}

// ParseSummary extracts the known Markdown sections without making parsing a
// prerequisite for using the raw checkpoint. Unknown sections are ignored so
// a newer summarizer remains readable by an older server.
func ParseSummary(markdown string) Summary {
	var summary Summary
	section := ""
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "-* "))
		if line == "" || line == "无明确记录" || line == "- 无明确记录" {
			continue
		}
		switch section {
		case "当前目标":
			summary.CurrentGoal = appendText(summary.CurrentGoal, line)
		case "用户偏好与约束":
			if strings.HasPrefix(line, "约束") || strings.HasPrefix(line, "Constraint") {
				summary.Constraints = appendLine(summary.Constraints, line)
			} else {
				summary.Preferences = appendLine(summary.Preferences, line)
			}
		case "关键决策":
			summary.Decisions = appendLine(summary.Decisions, line)
		case "已完成工作":
			summary.CompletedWork = appendLine(summary.CompletedWork, line)
		case "修改文件":
			summary.ModifiedFiles = appendLine(summary.ModifiedFiles, line)
		case "工具与验证":
			summary.ToolVerification = appendLine(summary.ToolVerification, line)
		case "问题与根因":
			summary.Problems = appendLine(summary.Problems, line)
		case "未完成任务与下一步":
			if strings.HasPrefix(line, "下一步") || strings.HasPrefix(line, "Next") {
				summary.NextSteps = appendLine(summary.NextSteps, line)
			} else {
				summary.OpenTasks = appendLine(summary.OpenTasks, line)
			}
		case "重要引用":
			summary.References = appendLine(summary.References, line)
		}
	}
	return summary
}

func appendText(existing, value string) string {
	if existing == "" {
		return value
	}
	return existing + "\n" + value
}

func appendLine(existing []string, value string) []string {
	return append(existing, value)
}
