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

	// 优先解析 Markdown 计划；解析不到时保留一个兜底步骤，避免产生不可执行的空 Plan。
	steps := ExtractSteps(content)
	if len(steps) == 0 {
		steps = []Step{{
			ID:     uuid.NewString(),
			Order:  1,
			Title:  summarizeLine(content, "Review request and propose next action"),
			Status: StepStatusPending,
		}}
	}

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
		lower := strings.ToLower(heading)
		if lower == "plan" || strings.Contains(lower, "plan") || strings.Contains(heading, "计划") || strings.Contains(heading, "规划") || strings.Contains(heading, "步骤") {
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
		lower := strings.ToLower(heading)
		if strings.Contains(lower, "result") ||
			strings.Contains(lower, "final") ||
			strings.Contains(lower, "output") ||
			strings.Contains(lower, "answer") ||
			strings.Contains(heading, "结果") ||
			strings.Contains(heading, "正文") ||
			strings.Contains(heading, "产出") ||
			strings.Contains(heading, "成品") ||
			strings.Contains(heading, "作品") {
			return true
		}
	}
	return false
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
