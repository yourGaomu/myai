package contextmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"

	domainmessage "myai/core/domain/message"
)

const (
	DefaultWindowK = 16
	MinWindowK     = 4
	MaxWindowK     = 256

	DefaultCompactTriggerRatio = 0.70
)

type Info struct {
	WindowK           int
	FullTokens        int
	SelectedTokens    int
	SummaryTokens     int
	PrefixTokens      int
	CacheableTokens   int
	FullMessages      int
	SelectedMessages  int
	CompactedMessages int
	HasSummary        bool
	Truncated         bool
	SummaryVersion    int
	SummaryHash       string
	PrefixHash        string
}

type Snapshot struct {
	// Prefix 是当前 runtime/user turn 之前的已选历史；Messages 是实际发送给模型的完整快照。
	Info     Info
	Messages []domainmessage.Message
	Prefix   []domainmessage.Message
}

func NormalizeWindowK(windowK int) int {
	if windowK <= 0 {
		return DefaultWindowK
	}
	return windowK
}

func ValidateWindowK(windowK int) error {
	if windowK < MinWindowK || windowK > MaxWindowK {
		return fmt.Errorf("context window must be between %dK and %dK", MinWindowK, MaxWindowK)
	}
	return nil
}

func BuildSnapshot(messages []domainmessage.Message, summary string, compactedMessages int, windowK int, fixedContext ...string) Snapshot {
	// base 始终放固定 system 和可选摘要，recent 再按 token 预算从新到旧选择完整消息块。
	summary = strings.TrimSpace(summary)
	summary = summaryForWindow(summary, windowK)
	base, recent := buildBaseAndRecent(messages, summary, compactedMessages, fixedContext...)
	info, selected := analyzePrepared(base, recent, messages, windowK, summary != "", EstimateTextTokens(summary), displayCompactedMessages(messages, compactedMessages))
	info.SummaryVersion = info.CompactedMessages
	info.SummaryHash = StableTextHash(summary)
	cacheablePrefix := stableCacheablePrefix(selected)
	info.PrefixTokens = EstimateMessagesTokens(cacheablePrefix)
	info.CacheableTokens = info.PrefixTokens
	info.PrefixHash = StableMessagesHash(cacheablePrefix)

	return Snapshot{
		Info:     info,
		Messages: selected,
		Prefix:   cacheablePrefix,
	}
}

func stableCacheablePrefix(messages []domainmessage.Message) []domainmessage.Message {
	lastUserIndex := -1
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == domainmessage.RoleUser {
			lastUserIndex = index
			break
		}
	}
	if lastUserIndex < 0 {
		return messages
	}
	turnStart := lastUserIndex
	for turnStart > 0 && messages[turnStart-1].IsSynthetic() {
		turnStart--
	}
	return messages[:turnStart]
}

func ShouldCompact(info Info, triggerRatio float64) bool {
	if info.WindowK <= 0 {
		return false
	}
	if triggerRatio <= 0 {
		triggerRatio = DefaultCompactTriggerRatio
	}
	budget := info.WindowK * 1000
	if budget <= 0 {
		return false
	}
	return info.Truncated || float64(info.SelectedTokens) >= float64(budget)*triggerRatio
}

func CompactSplit(messages []domainmessage.Message, compactedMessages int, keepChunks int) ([]domainmessage.Message, []domainmessage.Message, int) {
	if keepChunks <= 0 {
		keepChunks = 8
	}
	start := NormalizeCompactedMessages(messages, compactedMessages)
	if start >= len(messages) {
		return nil, nil, start
	}

	// 按“一次用户请求及其回答/工具结果”分块，避免摘要时切断 tool call 与 tool result。
	chunks := messageChunks(messages[start:])
	if len(chunks) <= keepChunks {
		return nil, messages[start:], start
	}

	compactChunkCount := len(chunks) - keepChunks
	compactableCount := 0
	for _, chunk := range chunks[:compactChunkCount] {
		compactableCount += len(chunk)
	}

	cutoff := start + compactableCount
	return messages[start:cutoff], messages[cutoff:], cutoff
}

func NormalizeCompactedMessages(messages []domainmessage.Message, compactedMessages int) int {
	if len(messages) == 0 {
		return 0
	}
	if compactedMessages < 1 {
		return 1
	}
	if compactedMessages > len(messages) {
		return len(messages)
	}
	return compactedMessages
}

func buildBaseAndRecent(messages []domainmessage.Message, summary string, compactedMessages int, fixedContext ...string) ([]domainmessage.Message, []domainmessage.Message) {
	system, rest := splitSystemMessage(messages)
	base := make([]domainmessage.Message, 0, len(system)+1+len(fixedContext))
	base = append(base, system...)

	summary = strings.TrimSpace(summary)
	if summary != "" {
		base = append(base, domainmessage.Text(domainmessage.RoleSystem, "Previous conversation summary:\n"+summary))
	}
	for _, item := range fixedContext {
		if item = strings.TrimSpace(item); item != "" {
			base = append(base, domainmessage.Text(domainmessage.RoleSystem, item))
		}
	}

	start := NormalizeCompactedMessages(messages, compactedMessages)
	if start > 1 {
		return base, messages[start:]
	}
	return base, rest
}

func analyzePrepared(base []domainmessage.Message, recent []domainmessage.Message, fullMessages []domainmessage.Message, windowK int, hasSummary bool, summaryTokens int, compactedMessages int) (Info, []domainmessage.Message) {
	windowK = NormalizeWindowK(windowK)
	if len(base) == 0 && len(recent) == 0 {
		return Info{WindowK: windowK, SummaryTokens: summaryTokens, CompactedMessages: compactedMessages, HasSummary: hasSummary}, nil
	}

	fullTokens := EstimateMessagesTokens(fullMessages)
	budget := windowK * 1000
	baseTokens := EstimateMessagesTokens(base)

	chunks := messageChunks(recent)
	selectedChunks := make([][]domainmessage.Message, 0, len(chunks))
	selectedTokens := baseTokens

	for i := len(chunks) - 1; i >= 0; i-- {
		chunk := chunks[i]
		chunkTokens := EstimateMessagesTokens(chunk)
		shouldInclude := selectedTokens+chunkTokens <= budget
		if !shouldInclude && len(selectedChunks) == 0 {
			shouldInclude = true
		}
		if !shouldInclude {
			break
		}

		selectedChunks = append(selectedChunks, chunk)
		selectedTokens += chunkTokens
	}

	selected := make([]domainmessage.Message, 0, len(base)+len(recent))
	selected = append(selected, base...)
	for i := len(selectedChunks) - 1; i >= 0; i-- {
		selected = append(selected, selectedChunks[i]...)
	}

	fullSelectedMessages := len(base) + len(recent)
	return Info{
		WindowK:           windowK,
		FullTokens:        fullTokens,
		SelectedTokens:    selectedTokens,
		SummaryTokens:     summaryTokens,
		FullMessages:      len(fullMessages),
		SelectedMessages:  len(selected),
		CompactedMessages: compactedMessages,
		HasSummary:        hasSummary,
		Truncated:         len(selected) < fullSelectedMessages || EstimateMessagesTokens(append(base, recent...)) > budget,
	}, selected
}

func displayCompactedMessages(messages []domainmessage.Message, compactedMessages int) int {
	limit := NormalizeCompactedMessages(messages, compactedMessages)
	count := 0
	for index := 0; index < limit; index++ {
		message := messages[index]
		if index == 0 && message.Role == domainmessage.RoleSystem && !message.IsSynthetic() {
			continue
		}
		if message.IsSynthetic() {
			continue
		}
		count++
	}
	return count
}

func EstimateMessagesTokens(messages []domainmessage.Message) int {
	total := 0
	for _, message := range messages {
		total += 4
		for _, part := range message.Parts {
			total += estimatePartTokens(part)
		}
	}
	return total
}

// CurrentTurnTokens returns the unavoidably selected tail beginning with the
// latest user message, including synthetic runtime context attached to it.
func CurrentTurnTokens(messages []domainmessage.Message) int {
	lastUserIndex := -1
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == domainmessage.RoleUser {
			lastUserIndex = index
			break
		}
	}
	if lastUserIndex < 0 {
		return 0
	}
	turnStart := lastUserIndex
	for turnStart > 0 && messages[turnStart-1].IsSynthetic() {
		turnStart--
	}
	return EstimateMessagesTokens(messages[turnStart:])
}

func StableMessagesHash(messages []domainmessage.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		builder.WriteString(string(message.Role))
		builder.WriteString("\n")
		if message.SyntheticReason != "" {
			builder.WriteString("synthetic:")
			builder.WriteString(string(message.SyntheticReason))
			builder.WriteString("\n")
		}
		for _, part := range message.Parts {
			writeStablePart(&builder, part)
			builder.WriteString("\n")
		}
		builder.WriteString("---\n")
	}
	return StableTextHash(builder.String())
}

func StableTextHash(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func splitSystemMessage(messages []domainmessage.Message) ([]domainmessage.Message, []domainmessage.Message) {
	if len(messages) == 0 || messages[0].Role != domainmessage.RoleSystem {
		return nil, messages
	}
	return messages[:1], messages[1:]
}

func messageChunks(messages []domainmessage.Message) [][]domainmessage.Message {
	chunks := make([][]domainmessage.Message, 0, len(messages))
	for start := 0; start < len(messages); {
		end := start + 1
		if messages[start].IsSynthetic() {
			for end < len(messages) && messages[end].IsSynthetic() {
				end++
			}
			if end < len(messages) && messages[end].Role == domainmessage.RoleUser {
				end++
			}
			for end < len(messages) && !messages[end].IsSynthetic() && messages[end].Role != domainmessage.RoleUser {
				end++
			}
		} else if messages[start].Role == domainmessage.RoleUser {
			for end < len(messages) && !messages[end].IsSynthetic() && messages[end].Role != domainmessage.RoleUser {
				end++
			}
		} else if messages[start].Role == domainmessage.RoleAssistant && messages[start].HasToolCall() {
			for end < len(messages) && messages[end].Role == domainmessage.RoleTool {
				end++
			}
		}
		chunks = append(chunks, messages[start:end])
		start = end
	}
	return chunks
}

func summaryForWindow(summary string, windowK int) string {
	windowTokens := NormalizeWindowK(windowK) * 1000
	limit := windowTokens / 4
	if limit > 1024 {
		limit = 1024
	}
	if limit < 256 {
		limit = 256
	}
	return TruncateTextToTokens(summary, limit)
}

// TruncateTextToTokens keeps durable decisions from the beginning and recent
// open work from the end when a summary must fit a strict context budget.
func TruncateTextToTokens(text string, maxTokens int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxTokens <= 0 {
		return ""
	}
	if EstimateTextTokens(text) <= maxTokens {
		return text
	}
	marker := "\n[...context truncated...]\n"
	markerTokens := EstimateTextTokens(marker)
	if maxTokens <= markerTokens+2 {
		return truncatePrefixToTokens(text, maxTokens)
	}
	contentBudget := maxTokens - markerTokens
	head := truncatePrefixToTokens(text, contentBudget*3/5)
	tail := truncateSuffixToTokens(text, contentBudget-EstimateTextTokens(head))
	return strings.TrimSpace(head) + marker + strings.TrimSpace(tail)
}

func truncatePrefixToTokens(text string, maxTokens int) string {
	runes := []rune(text)
	low, high := 0, len(runes)
	for low < high {
		middle := (low + high + 1) / 2
		if EstimateTextTokens(string(runes[:middle])) <= maxTokens {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return string(runes[:low])
}

func truncateSuffixToTokens(text string, maxTokens int) string {
	runes := []rune(text)
	low, high := 0, len(runes)
	for low < high {
		middle := (low + high + 1) / 2
		if EstimateTextTokens(string(runes[len(runes)-middle:])) <= maxTokens {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return string(runes[len(runes)-low:])
}

func estimatePartTokens(part domainmessage.Part) int {
	switch part.Type {
	case domainmessage.PartText:
		return EstimateTextTokens(part.Text)
	case domainmessage.PartToolCall:
		if part.ToolCall == nil {
			return 0
		}
		text := part.ToolCall.ID + " " + part.ToolCall.Type + " " + part.ToolCall.Name + " " + part.ToolCall.Arguments
		return EstimateTextTokens(text)
	case domainmessage.PartToolResult:
		if part.ToolResult == nil {
			return 0
		}
		return EstimateTextTokens(part.ToolResult.ToolCallID + " " + part.ToolResult.Name + " " + part.ToolResult.PromptContent())
	default:
		return EstimateTextTokens(fmt.Sprint(part))
	}
}

func writeStablePart(builder *strings.Builder, part domainmessage.Part) {
	switch part.Type {
	case domainmessage.PartText:
		builder.WriteString("text:")
		builder.WriteString(part.Text)
	case domainmessage.PartToolCall:
		if part.ToolCall == nil {
			return
		}
		builder.WriteString("tool_call:")
		builder.WriteString(part.ToolCall.ID)
		builder.WriteString(":")
		builder.WriteString(part.ToolCall.Type)
		builder.WriteString(":")
		builder.WriteString(part.ToolCall.Name)
		builder.WriteString(":")
		builder.WriteString(part.ToolCall.Arguments)
	case domainmessage.PartToolResult:
		if part.ToolResult == nil {
			return
		}
		builder.WriteString("tool_result:")
		builder.WriteString(part.ToolResult.ToolCallID)
		builder.WriteString(":")
		builder.WriteString(part.ToolResult.Name)
		builder.WriteString(":")
		builder.WriteString(part.ToolResult.Content)
	default:
		builder.WriteString(fmt.Sprint(part))
	}
}

func EstimateTextTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}

	ascii := 0
	nonASCII := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		if r <= unicode.MaxASCII {
			ascii++
			continue
		}
		nonASCII++
	}

	tokens := (ascii + 3) / 4
	tokens += nonASCII
	if tokens == 0 {
		return 1
	}
	return tokens
}
