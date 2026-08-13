package plan

import (
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
	ID         string
	SessionID  string
	Goal       string
	Status     string
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
	Status      string
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

func Clone(p *Plan) *Plan {
	if p == nil {
		return nil
	}
	next := *p
	if len(p.Steps) > 0 {
		next.Steps = append([]Step(nil), p.Steps...)
	}
	return &next
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
