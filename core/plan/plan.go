package plan

import (
	"encoding/json"
	"regexp"
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
	if steps := extractStructuredSteps(content); len(steps) > 0 {
		return steps
	}
	// 只提取 Plan/计划标题下的列表项，最多 12 步，防止模型输出被无限扩张为执行任务。
	lines := planSectionLines(content)
	steps := make([]Step, 0, 8)
	for _, line := range lines {
		match := stepLinePattern.FindStringSubmatch(line)
		if len(match) < 2 {
			continue
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
		if len(steps) >= 12 {
			break
		}
	}
	return steps
}

// extractStructuredSteps accepts the machine-readable plan shape emitted by
// newer models while retaining Markdown parsing as a backwards-compatible
// fallback for existing sessions.
func extractStructuredSteps(content string) []Step {
	trimmed := strings.TrimSpace(content)
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			trimmed = trimmed[start : end+1]
		}
	}
	var payload struct {
		Steps []struct {
			ID           string   `json:"id"`
			Order        int      `json:"order"`
			Title        string   `json:"title"`
			Description  string   `json:"description"`
			DependsOn    []string `json:"depends_on"`
			Dependencies []string `json:"dependencies"`
			Status       string   `json:"status"`
			MaxRetries   int      `json:"max_retries"`
		} `json:"steps"`
	}
	if json.Unmarshal([]byte(trimmed), &payload) != nil || len(payload.Steps) == 0 {
		return nil
	}
	steps := make([]Step, 0, len(payload.Steps))
	for index, item := range payload.Steps {
		title := summarizeLine(item.Title, "")
		if title == "" {
			continue
		}
		status := item.Status
		if status == "" {
			status = StepStatusPending
		}
		order := item.Order
		if order <= 0 {
			order = len(steps) + 1
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = uuid.NewString()
		}
		dependencies := append([]string(nil), item.DependsOn...)
		if len(dependencies) == 0 {
			dependencies = append(dependencies, item.Dependencies...)
		}
		steps = append(steps, Step{ID: id, Order: order, Title: title,
			Description: strings.TrimSpace(item.Description), Dependencies: normalizeDependencies(dependencies),
			Status: status, MaxRetries: item.MaxRetries})
		if index >= 11 {
			break
		}
	}
	return steps
}

func normalizeDependencies(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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

func planSectionLines(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start := -1
	for index, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmedLine, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(trimmedLine, "#"))
		heading = strings.Trim(heading, " :：")
		if isPlanHeading(heading) {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return lines
	}

	selected := make([]string, 0, len(lines)-start)
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if len(selected) > 0 && strings.HasPrefix(trimmed, "#") {
			break
		}
		selected = append(selected, line)
	}
	return selected
}

func HasResultSection(content string) bool {
	// 安全的纯文本任务会在同一回复中包含 Result；这种计划可直接标记完成，无需再次点击执行。
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		heading = strings.Trim(heading, " :：")
		if isResultHeading(heading) {
			return true
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
	case "result", "final result", "output", "final output", "answer", "final answer",
		"结果", "最终结果", "正文", "产出", "成品", "作品":
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
