package plan

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	StatusDraft    = "draft"
	StatusApproved = "approved"
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"

	StepStatusPending = "pending"
	StepStatusRunning = "running"
	StepStatusDone    = "done"
	StepStatusFailed  = "failed"
	StepStatusSkipped = "skipped"
)

type Plan struct {
	// RawContent 保留模型原始回复，Steps 是供状态机和手机界面使用的结构化结果。
	ID        string
	SessionID string
	Goal      string
	Status    string
	// Revision increases whenever a plan snapshot is persisted. It lets
	// clients order updates even when transport events arrive out of order.
	Revision   int64
	RawContent string
	Steps      []Step
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Step struct {
	ID          string
	Order       int
	Title       string
	Description string
	// Dependencies contains step IDs (or legacy order numbers) that must be
	// completed before this step becomes runnable.
	Dependencies []string
	Status       string
	RetryCount   int
	MaxRetries   int
	LastError    string
	AgentTaskID  string
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

var stepLinePattern = regexp.MustCompile(`^\s*(?:[-*]\s+\[[ xX-]\]\s+|[-*]\s+|\d+[\.)]\s+)(.+?)\s*$`)

func NewDraft(sessionID string, goal string, content string, now time.Time) *Plan {
	if now.IsZero() {
		now = time.Now()
	}

	steps := ExtractSteps(content)

	return &Plan{
		ID:         uuid.NewString(),
		SessionID:  strings.TrimSpace(sessionID),
		Goal:       strings.TrimSpace(goal),
		Status:     StatusDraft,
		RawContent: strings.TrimSpace(content),
		Steps:      steps,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func IsExecutableStatus(status string) bool {
	switch status {
	case StatusDraft, StatusApproved, StatusRunning, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}

func ExtractSteps(content string) []Step {
	if steps, structured := extractStructuredSteps(content); structured {
		return steps
	}
	// Only the first list level in an explicit Plan section is executable.
	lines := planSectionLines(content)
	steps := make([]Step, 0, 8)
	listIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if listIndent >= 0 && indent > listIndent {
			continue
		}
		match := stepLinePattern.FindStringSubmatch(line)
		if len(match) < 2 {
			if listIndent >= 0 {
				break
			}
			continue
		}
		if listIndent < 0 {
			listIndent = indent
		}
		title, description := splitStepText(match[1])
		if title == "" {
			continue
		}
		steps = append(steps, Step{
			ID:          uuid.NewString(),
			Order:       len(steps) + 1,
			Title:       title,
			Description: description,
			Status:      StepStatusPending,
		})
		if len(steps) > 12 {
			return nil
		}
	}
	return steps
}

// extractStructuredSteps accepts the machine-readable plan shape emitted by
// newer models while retaining Markdown parsing as a backwards-compatible
// fallback for existing sessions.
func extractStructuredSteps(content string) ([]Step, bool) {
	trimmed := strings.TrimSpace(content)
	structured := strings.HasPrefix(trimmed, "{")
	if !structured {
		lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
		for index, line := range lines {
			if strings.TrimSpace(line) != "```json" {
				continue
			}
			structured = true
			for end := index + 1; end < len(lines); end++ {
				if strings.TrimSpace(lines[end]) == "```" {
					trimmed = strings.TrimSpace(strings.Join(lines[index+1:end], "\n"))
					break
				}
			}
			break
		}
	}
	if !structured {
		return nil, false
	}
	var payload struct {
		Steps []struct {
			ID           string   `json:"id"`
			Order        *int     `json:"order"`
			Title        string   `json:"title"`
			Description  string   `json:"description"`
			DependsOn    []string `json:"depends_on"`
			Dependencies []string `json:"dependencies"`
			MaxRetries   int      `json:"max_retries"`
		} `json:"steps"`
	}
	if json.Unmarshal([]byte(trimmed), &payload) != nil || len(payload.Steps) == 0 || len(payload.Steps) > 12 {
		return nil, true
	}
	steps := make([]Step, 0, len(payload.Steps))
	for index, item := range payload.Steps {
		title := summarizeLine(item.Title, "")
		if title == "" {
			return nil, true
		}
		order := index + 1
		if item.Order != nil {
			order = *item.Order
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = uuid.NewString()
		}
		dependencies := append(append([]string(nil), item.DependsOn...), item.Dependencies...)
		steps = append(steps, Step{ID: id, Order: order, Title: title,
			Description: strings.TrimSpace(item.Description), Dependencies: normalizeDependencies(dependencies),
			Status: StepStatusPending, MaxRetries: item.MaxRetries})
	}
	return steps, true
}

func normalizeDependencies(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// Validate checks the whole dependency graph before callers publish state or
// execute work. Legacy numeric order references remain valid only when unique.
func Validate(p *Plan) error {
	if p == nil || len(p.Steps) == 0 {
		return fmt.Errorf("plan has no steps")
	}
	if len(p.Steps) > 12 {
		return fmt.Errorf("plan exceeds the 12 step limit")
	}
	ids := make(map[string]bool, len(p.Steps))
	orders := make(map[int]bool, len(p.Steps))
	for index, step := range p.Steps {
		if step.ID == "" || step.ID != strings.TrimSpace(step.ID) {
			return fmt.Errorf("step %d has an empty or invalid ID", index+1)
		}
		if ids[step.ID] {
			return fmt.Errorf("duplicate step ID %q", step.ID)
		}
		ids[step.ID] = true
		if step.Order <= 0 || orders[step.Order] {
			return fmt.Errorf("step %q has an invalid or duplicate order %d", step.ID, step.Order)
		}
		orders[step.Order] = true
		if strings.TrimSpace(step.Title) == "" {
			return fmt.Errorf("step %q has no title", step.ID)
		}
		switch step.Status {
		case "", StepStatusPending, StepStatusRunning, StepStatusDone, StepStatusFailed, StepStatusSkipped:
		default:
			return fmt.Errorf("step %q has an invalid status", step.ID)
		}
	}
	edges := make([][]int, len(p.Steps))
	for index, step := range p.Steps {
		for _, reference := range step.Dependencies {
			dependency, err := DependencyIndex(p, reference)
			if err != nil {
				return fmt.Errorf("step %q: %w", step.ID, err)
			}
			if dependency == index {
				return fmt.Errorf("step %q has a self dependency", step.ID)
			}
			edges[index] = append(edges[index], dependency)
		}
	}
	visiting := make([]uint8, len(p.Steps))
	var visit func(int) error
	visit = func(index int) error {
		if visiting[index] == 1 {
			return fmt.Errorf("plan has a cyclic step dependency at %q", p.Steps[index].ID)
		}
		if visiting[index] == 2 {
			return nil
		}
		visiting[index] = 1
		for _, dependency := range edges[index] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[index] = 2
		return nil
	}
	for index := range p.Steps {
		if err := visit(index); err != nil {
			return err
		}
	}
	return nil
}

// DependencyIndex resolves an ID or a legacy order reference, rejecting aliases
// that identify more than one step rather than selecting the first match.
func DependencyIndex(p *Plan, reference string) (int, error) {
	reference = strings.TrimSpace(reference)
	if p == nil || reference == "" {
		return -1, fmt.Errorf("empty step dependency")
	}
	found := -1
	for index, step := range p.Steps {
		if step.ID != reference && strconv.Itoa(step.Order) != reference {
			continue
		}
		if found >= 0 {
			return -1, fmt.Errorf("ambiguous step dependency %q", reference)
		}
		found = index
	}
	if found < 0 {
		return -1, fmt.Errorf("unknown step dependency %q", reference)
	}
	return found, nil
}

func Clone(p *Plan) *Plan {
	if p == nil {
		return nil
	}
	next := *p
	if len(p.Steps) > 0 {
		next.Steps = append([]Step(nil), p.Steps...)
		for index := range next.Steps {
			next.Steps[index].Dependencies = append([]string(nil), p.Steps[index].Dependencies...)
			next.Steps[index].StartedAt = cloneTime(next.Steps[index].StartedAt)
			next.Steps[index].CompletedAt = cloneTime(next.Steps[index].CompletedAt)
		}
	}
	return &next
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func markdownLines(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	fence := ""
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			lines[index] = ""
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
			lines[index] = ""
		}
	}
	return lines
}

func markdownHeading(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return ""
	}
	heading := strings.TrimLeft(trimmed, "#")
	if len(trimmed)-len(heading) > 6 || (heading != "" && heading[0] != ' ' && heading[0] != '\t') {
		return ""
	}
	return strings.Trim(strings.TrimSpace(heading), " #\t:：")
}

func planSectionLines(content string) []string {
	lines := markdownLines(content)
	start := -1
	for index, line := range lines {
		if isPlanHeading(markdownHeading(line)) {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	for end := start; end < len(lines); end++ {
		if markdownHeading(lines[end]) != "" {
			return lines[start:end]
		}
	}
	return lines[start:]
}

// HasResultSection identifies a standalone, nonempty Result answer. A reply
// containing a Plan or structured steps must still pass through execution;
// a heading alone is never evidence that its actions have completed.
func HasResultSection(content string) bool {
	if _, structured := extractStructuredSteps(content); structured {
		return false
	}
	lines := markdownLines(content)
	for _, line := range lines {
		if isPlanHeading(markdownHeading(line)) {
			return false
		}
	}
	for index, line := range lines {
		if !isResultHeading(markdownHeading(line)) {
			continue
		}
		for _, body := range lines[index+1:] {
			if markdownHeading(body) != "" {
				break
			}
			if strings.TrimSpace(body) != "" {
				return true
			}
		}
	}
	return false
}

func isPlanHeading(heading string) bool {
	switch strings.ToLower(strings.Join(strings.Fields(heading), " ")) {
	case "plan", "execution plan", "implementation plan", "proposed plan",
		"计划", "执行计划", "执行规划", "规划", "步骤":
		return true
	default:
		return false
	}
}

func isResultHeading(heading string) bool {
	switch strings.ToLower(strings.Join(strings.Fields(heading), " ")) {
	case "result", "final result", "结果", "最终结果":
		return true
	default:
		return false
	}
}

func splitStepText(text string) (string, string) {
	cleaned := strings.TrimSpace(text)
	cleaned = strings.Trim(cleaned, "*_` ")
	cleaned = strings.ReplaceAll(cleaned, "**", "")
	if cleaned == "" {
		return "", ""
	}

	for _, sep := range []string{" - ", "：", ": "} {
		if before, after, ok := strings.Cut(cleaned, sep); ok {
			title := summarizeLine(before, "")
			return title, strings.TrimSpace(after)
		}
	}
	return summarizeLine(cleaned, ""), ""
}

func summarizeLine(text string, fallback string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return fallback
	}
	if utf8.RuneCountInString(text) <= 96 {
		return text
	}

	runes := []rune(text)
	return strings.TrimSpace(string(runes[:96])) + "..."
}
