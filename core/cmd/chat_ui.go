package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"myai/core/llm"
	"myai/core/service"
	"myai/core/skill"
	"myai/core/store/data"
)

const (
	defaultUIWidth = 88
	minUIWidth     = 64
	maxUIWidth     = 120
	contentPrefix  = "    "
)

var (
	brandStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("42")).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	bodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))

	roleUserStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("42")).
			Padding(0, 1)

	roleAIStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("63")).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("203"))

	tokenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("110"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	reasoningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)

	reasoningLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)

	toolStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("214")).
			Padding(0, 1)

	commandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81"))

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("250"))

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))
)

func printChatHeader(sessionID string, modelID string) {
	width := currentUIWidth()
	fmt.Println()
	fmt.Println(brandStyle.Render("MYAI") + " " + titleStyle.Render("AI coding assistant"))
	meta := []string{"session " + shortID(sessionID)}
	if modelID != "" {
		meta = append(meta, "model "+modelID)
	}
	meta = append(meta, "/help commands", "/exit quit")
	fmt.Println(mutedStyle.Render(strings.Join(meta, "  |  ")))
	printDivider(width)
	fmt.Println()
}

func printPrompt() {
	fmt.Print(promptStyle.Render("myai") + mutedStyle.Render(" >") + " ")
}

func printTurnDivider() {
	fmt.Println()
	printDivider(currentUIWidth())
}

func printUserInput(input string) {
	fmt.Println(roleUserStyle.Render("You"))
	printBlockText(input)
	fmt.Println()
}

func printAssistantHeader() {
	fmt.Println(roleAIStyle.Render("Assistant"))
}

func newChatStreamHandler(reader *bufio.Scanner) llm.ChatStreamHandler {
	reasoningStarted := false
	answerStarted := false
	toolStarted := false
	reasoningPrinter := newStreamPrinter(reasoningStyle)
	answerPrinter := newStreamPrinter(bodyStyle)

	return llm.ChatStreamHandler{
		OnReasoning: func(text string) {
			if !reasoningStarted {
				printReasoningHeader()
				reasoningStarted = true
			}
			reasoningPrinter.Print(text)
		},
		OnToolCall: func(name string, arguments string) {
			reasoningPrinter.Finish()
			answerPrinter.Finish()
			printToolCall(name, arguments)
			toolStarted = true
		},
		OnToolResult: func(name string, arguments string, result string) {
			reasoningPrinter.Finish()
			answerPrinter.Finish()
			printToolResult(name, result)
			toolStarted = true
		},
		OnToolAsk: func(request llm.ToolPermissionRequest) bool {
			reasoningPrinter.Finish()
			answerPrinter.Finish()
			return confirmToolPermission(reader, request)
		},
		OnAnswer: func(text string) {
			if !answerStarted {
				reasoningPrinter.Finish()
				if reasoningStarted || toolStarted {
					printAnswerHeader()
				}
				answerStarted = true
			}
			answerPrinter.Print(text)
		},
	}
}

func printReasoningHeader() {
	fmt.Println(reasoningLabelStyle.Render("Thinking"))
}

func printAnswerHeader() {
	fmt.Println(roleAIStyle.Render("Answer"))
}

func printToolCall(name string, arguments string) {
	fmt.Println(toolStyle.Render("Tool") + " " + commandStyle.Render(name))

	arguments = compactLine(arguments)
	if arguments != "" {
		printWrappedText(truncate(arguments, currentContentWidth()), mutedStyle)
	}
}

func printToolResult(name string, result string) {
	label := "Tool result"
	if strings.Contains(strings.ToLower(result), "tool error:") {
		label = "Tool error"
	}
	fmt.Println(toolStyle.Render(label) + " " + commandStyle.Render(name))

	result = strings.TrimSpace(result)
	if result != "" {
		printWrappedText(truncate(result, currentContentWidth()), mutedStyle)
	}
}

func printResponseFooter(sessionID string, usage llm.TokenUsage, contextInfo service.ContextInfo, compactInfo service.CompactInfo) {
	width := currentUIWidth()
	fmt.Println()

	parts := make([]string, 0, 4)
	if sessionID != "" {
		parts = append(parts, "session "+shortID(sessionID))
	}
	parts = append(parts, contextSummary(contextInfo))
	if compactInfo.Triggered {
		parts = append(parts, compactSummary(compactInfo))
	}
	parts = append(parts, tokenSummary(usage))

	printDivider(width)
	printFooterParts(parts, width)
	fmt.Println()
}

func printSuccess(message string) {
	fmt.Println(statusLine("ok", message, successStyle))
}

func printWarning(message string) {
	fmt.Println(statusLine("warn", message, warnStyle))
}

func printError(prefix string, err error) {
	if err == nil {
		fmt.Println(statusLine("error", prefix, errorStyle))
		return
	}
	fmt.Println(statusLine("error", prefix+" "+err.Error(), errorStyle))
}

func printChatHelp() {
	printSectionTitle("Commands")
	printCommand("/help", "Show this help")
	printCommand("/new", "Create and switch to a new session")
	printCommand("/sessions", "List saved sessions")
	printCommand("/use <id>", "Switch to a saved session")
	printCommand("/skills", "Reload and list local skills")
	printCommand("/permission", "Show session permission mode")
	printCommand("/permission <mode>", "Set readonly, ask, or full")
	printCommand("/context", "Show context window usage")
	printCommand("/context <K>", "Set session context window")
	printCommand("/compact", "Summarize older context")
	printCommand("/models", "List available models")
	printCommand("/model", "Show current model")
	printCommand("/model add", "Add a model config")
	printCommand("/model <id>", "Switch current session model")
	printCommand("/clear", "Clear current session messages")
	printCommand("/exit", "Leave chat")
	fmt.Println()
}

func printCommand(name string, description string) {
	fmt.Printf("%s %s\n", commandStyle.Render(padRight(name, 16)), mutedStyle.Render(description))
}

func printTokenUsage(usage llm.TokenUsage) {
	fmt.Println(statusLine("usage", tokenSummary(usage), tokenStyle))
}

func printSessionsTable(sessions []data.SessionRecord, currentID string) {
	if len(sessions) == 0 {
		printWarning("no saved sessions.")
		return
	}

	titleWidth := clampInt(currentUIWidth()-35, 18, 42)
	printSectionTitle("Sessions")
	fmt.Println(tableHeaderStyle.Render(
		padRight("", 3) +
			padRight("session", 12) +
			padRight("title", titleWidth+2) +
			"updated",
	))
	for _, session := range sessions {
		marker := " "
		if session.ID == currentID {
			marker = ">"
		}

		line := fmt.Sprintf(
			"%s  %s  %s  %s",
			marker,
			padRight(shortID(session.ID), 10),
			padRight(truncate(session.Title, titleWidth), titleWidth+2),
			session.UpdatedAt.Format(time.DateTime),
		)
		if session.ID == currentID {
			fmt.Println(selectedRowStyle.Render(line))
			continue
		}
		fmt.Println(bodyStyle.Render(line))
	}
	fmt.Println()
}

func printModelsTable(models []llm.ModelInfo, currentID string) {
	if len(models) == 0 {
		printWarning("no available models.")
		return
	}

	modelWidth := clampInt((currentUIWidth()-35)/2, 14, 28)
	printSectionTitle("Models")
	fmt.Println(tableHeaderStyle.Render(
		padRight("", 3) +
			padRight("id", modelWidth+2) +
			padRight("provider", 14) +
			padRight("model", modelWidth+2) +
			"flags",
	))
	for _, model := range models {
		marker := " "
		if model.ID == currentID {
			marker = ">"
		}

		provider := model.Provider
		if provider == "" {
			provider = "-"
		}

		flags := ""
		if model.IsDefault {
			flags = "default"
		}

		line := fmt.Sprintf(
			"%s  %s  %s  %s  %s",
			marker,
			padRight(truncate(model.ID, modelWidth), modelWidth),
			padRight(truncate(provider, 12), 12),
			padRight(truncate(model.ModelName, modelWidth), modelWidth),
			flags,
		)
		if model.ID == currentID {
			fmt.Println(selectedRowStyle.Render(line))
			continue
		}
		fmt.Println(bodyStyle.Render(line))
	}
	fmt.Println()
}

func printSkillsTable(skills []skill.Skill, root string) {
	printSectionTitle("Skills")
	if root != "" {
		fmt.Println(statusLine("root", root, mutedStyle))
	}
	if len(skills) == 0 {
		printWarning("no skills loaded. create skills/<name>/SKILL.md to add one.")
		return
	}

	width := currentUIWidth()
	nameWidth := clampInt((width-45)/2, 12, 28)
	descriptionWidth := clampInt(width-nameWidth-35, 18, 42)
	fmt.Println(tableHeaderStyle.Render(
		padRight("name", nameWidth+2) +
			padRight("description", descriptionWidth+2) +
			padRight("updated", 18) +
			"path",
	))
	for _, item := range skills {
		updated := "-"
		if !item.UpdatedAt.IsZero() {
			updated = item.UpdatedAt.Format("2006-01-02 15:04")
		}
		line := fmt.Sprintf(
			"%s  %s  %s  %s",
			padRight(truncate(item.Name, nameWidth), nameWidth),
			padRight(truncate(item.Description, descriptionWidth), descriptionWidth),
			padRight(updated, 16),
			truncate(item.Path, maxInt(12, width-nameWidth-descriptionWidth-26)),
		)
		fmt.Println(bodyStyle.Render(line))
	}
	fmt.Println()
}

func printPermissionMode(mode any) {
	printSectionTitle("Permission")
	fmt.Println(statusLine("mode", fmt.Sprint(mode), successStyle))
	fmt.Println(contentPrefix + mutedStyle.Render("readonly = read tools only"))
	fmt.Println(contentPrefix + mutedStyle.Render("ask      = confirm write and execute tools"))
	fmt.Println(contentPrefix + mutedStyle.Render("full     = allow all tools"))
	fmt.Println()
}

func printContextWindow(info service.ContextInfo) {
	printSectionTitle("Context")
	fmt.Println(statusLine("window", fmt.Sprintf("%dK", info.WindowK), successStyle))
	printWrappedText(contextDetail(info), mutedStyle)
	fmt.Println()
}

func confirmToolPermission(reader *bufio.Scanner, request llm.ToolPermissionRequest) bool {
	printSectionTitle("Permission required")
	fmt.Println(statusLine("tool", request.Name, commandStyle))
	fmt.Println(statusLine("mode", request.Mode, mutedStyle))
	fmt.Println(statusLine("need", string(request.Permission), warnStyle))

	arguments := compactLine(request.Arguments)
	if arguments != "" {
		fmt.Println(statusLine("args", truncate(arguments, currentContentWidth()), mutedStyle))
	}

	for {
		fmt.Print(commandStyle.Render("allow") + mutedStyle.Render(" [y/N] "))
		if reader == nil || !reader.Scan() {
			return false
		}

		answer := strings.TrimSpace(strings.ToLower(reader.Text()))
		switch answer {
		case "y", "yes":
			return true
		case "", "n", "no":
			return false
		default:
			printWarning("please answer y or n.")
		}
	}
}

func printModelAddHeader() {
	printSectionTitle("Add model")
	fmt.Println(contentPrefix + mutedStyle.Render("type /cancel to stop"))
}

func printFormPrompt(label string, defaultValue string) {
	text := label
	if defaultValue != "" {
		text += " " + mutedStyle.Render("["+defaultValue+"]")
	}
	fmt.Print(commandStyle.Render(text) + mutedStyle.Render(" >") + " ")
}

func indentText(text string) string {
	lines := wrapText(text, currentContentWidth())
	for i := range lines {
		lines[i] = contentPrefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func printBlockText(text string) {
	printWrappedText(text, bodyStyle)
}

func printWrappedText(text string, style lipgloss.Style) {
	for _, line := range wrapText(text, currentContentWidth()) {
		fmt.Println(contentPrefix + style.Render(line))
	}
}

func printSectionTitle(title string) {
	fmt.Println(titleStyle.Render(title))
	printDivider(minInt(currentUIWidth(), 56))
}

func printDivider(width int) {
	fmt.Println(dividerStyle.Render(strings.Repeat("-", clampInt(width, minUIWidth, maxUIWidth))))
}

func printFooterParts(parts []string, width int) {
	line := ""
	separator := "  |  "
	for _, part := range parts {
		next := part
		if line != "" {
			next = line + separator + part
		}
		if line != "" && lipgloss.Width(next) > width {
			fmt.Println(footerStyle.Render(line))
			line = part
			continue
		}
		line = next
	}
	if line != "" {
		fmt.Println(footerStyle.Render(line))
	}
}

func statusLine(label string, value string, style lipgloss.Style) string {
	return contentPrefix + mutedStyle.Render(padRight(label, 8)) + style.Render(value)
}

func currentUIWidth() int {
	width, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || width <= 0 {
		width = defaultUIWidth
	}
	return clampInt(width-2, minUIWidth, maxUIWidth)
}

func currentContentWidth() int {
	return maxInt(24, currentUIWidth()-lipgloss.Width(contentPrefix))
}

func wrapText(text string, width int) []string {
	width = maxInt(1, width)
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, rawLine := range rawLines {
		if rawLine == "" {
			lines = append(lines, "")
			continue
		}

		remaining := rawLine
		for lipgloss.Width(remaining) > width {
			cut := wrapCutIndex(remaining, width)
			lines = append(lines, strings.TrimRight(remaining[:cut], " "))
			remaining = strings.TrimLeft(remaining[cut:], " ")
			if remaining == "" {
				break
			}
		}
		if remaining != "" {
			lines = append(lines, remaining)
		}
	}
	return lines
}

func wrapCutIndex(text string, width int) int {
	lastSpace := -1
	currentWidth := 0
	for index, r := range text {
		if r == ' ' || r == '\t' {
			lastSpace = index
		}

		currentWidth += lipgloss.Width(string(r))
		if currentWidth > width {
			if lastSpace > 0 {
				return lastSpace + 1
			}
			if index > 0 {
				return index
			}
			return len(string(r))
		}
	}
	return len(text)
}

func clampInt(value int, minValue int, maxValue int) int {
	return maxInt(minValue, minInt(value, maxValue))
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func truncate(text string, maxLength int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return "New chat"
	}
	if len(runes) <= maxLength {
		return string(runes)
	}
	if maxLength <= 3 {
		return string(runes[:maxLength])
	}
	return string(runes[:maxLength-3]) + "..."
}

func padRight(text string, width int) string {
	if lipgloss.Width(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-lipgloss.Width(text))
}

func compactLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func contextSummary(info service.ContextInfo) string {
	status := ""
	if info.Truncated {
		status = " truncated"
	}
	if info.HasSummary {
		status += " summary"
	}
	return fmt.Sprintf("ctx %d/%d tokens window %dK%s", info.SelectedTokens, info.FullTokens, info.WindowK, status)
}

func contextDetail(info service.ContextInfo) string {
	status := "full context fits in window"
	if info.Truncated {
		status = "older messages will be omitted from model input"
	}
	if info.HasSummary {
		status += ", summary enabled"
	}

	return fmt.Sprintf(
		"selected %d/%d tokens, messages %d/%d, compacted %d, summary %d tokens, %s",
		info.SelectedTokens,
		info.FullTokens,
		info.SelectedMessages,
		info.FullMessages,
		info.CompactedMessages,
		info.SummaryTokens,
		status,
	)
}

func compactSummary(info service.CompactInfo) string {
	return fmt.Sprintf(
		"compacted %d msgs ctx %d->%d summary %d",
		info.NewMessages,
		info.BeforeTokens,
		info.AfterTokens,
		info.SummaryTokens,
	)
}

func tokenSummary(usage llm.TokenUsage) string {
	if !usage.Available {
		return "tokens unavailable"
	}

	text := fmt.Sprintf(
		"tokens input %d output %d total %d",
		usage.PromptTokens,
		usage.CompletionTokens,
		usage.TotalTokens,
	)
	if usage.ReasoningTokens > 0 {
		text += fmt.Sprintf(" reasoning %d", usage.ReasoningTokens)
	}
	if usage.PromptCachedTokens > 0 {
		text += fmt.Sprintf(" cached %d", usage.PromptCachedTokens)
	}

	return text
}

type streamPrinter struct {
	prefix      string
	style       lipgloss.Style
	atLineStart bool
}

func newStreamPrinter(style lipgloss.Style) *streamPrinter {
	return &streamPrinter{
		prefix:      contentPrefix,
		style:       style,
		atLineStart: true,
	}
}

func (p *streamPrinter) Print(text string) {
	for text != "" {
		if p.atLineStart {
			fmt.Print(p.prefix)
			p.atLineStart = false
		}

		newlineIndex := strings.IndexByte(text, '\n')
		if newlineIndex < 0 {
			fmt.Print(p.style.Render(text))
			return
		}

		fmt.Print(p.style.Render(text[:newlineIndex]))
		fmt.Println()
		p.atLineStart = true
		text = text[newlineIndex+1:]
	}
}

func (p *streamPrinter) Finish() {
	if !p.atLineStart {
		fmt.Println()
		p.atLineStart = true
	}
}
