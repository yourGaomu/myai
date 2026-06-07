package contextmgr

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/tmc/langchaingo/llms"
)

const (
	DefaultWindowK = 16
	MinWindowK     = 4
	MaxWindowK     = 256
)

type Info struct {
	WindowK           int
	FullTokens        int
	SelectedTokens    int
	SummaryTokens     int
	FullMessages      int
	SelectedMessages  int
	CompactedMessages int
	HasSummary        bool
	Truncated         bool
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

func Build(messages []llms.MessageContent, windowK int) []llms.MessageContent {
	return BuildWithSummary(messages, "", 0, windowK)
}

func BuildWithSummary(messages []llms.MessageContent, summary string, compactedMessages int, windowK int) []llms.MessageContent {
	info, selected := AnalyzeWithSummary(messages, summary, compactedMessages, windowK)
	if info.Truncated {
		return selected
	}
	return selected
}

func Analyze(messages []llms.MessageContent, windowK int) (Info, []llms.MessageContent) {
	return AnalyzeWithSummary(messages, "", 0, windowK)
}

func AnalyzeWithSummary(messages []llms.MessageContent, summary string, compactedMessages int, windowK int) (Info, []llms.MessageContent) {
	summary = strings.TrimSpace(summary)
	base, recent := buildBaseAndRecent(messages, summary, compactedMessages)
	return analyzePrepared(base, recent, messages, windowK, summary != "", EstimateTextTokens(summary), displayCompactedMessages(messages, compactedMessages))
}

func CompactSplit(messages []llms.MessageContent, compactedMessages int, keepChunks int) ([]llms.MessageContent, []llms.MessageContent, int) {
	if keepChunks <= 0 {
		keepChunks = 8
	}
	start := NormalizeCompactedMessages(messages, compactedMessages)
	if start >= len(messages) {
		return nil, nil, start
	}

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

func NormalizeCompactedMessages(messages []llms.MessageContent, compactedMessages int) int {
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

func buildBaseAndRecent(messages []llms.MessageContent, summary string, compactedMessages int) ([]llms.MessageContent, []llms.MessageContent) {
	system, rest := splitSystemMessage(messages)
	base := make([]llms.MessageContent, 0, len(system)+1)
	base = append(base, system...)

	summary = strings.TrimSpace(summary)
	if summary != "" {
		base = append(base, llms.TextParts(llms.ChatMessageTypeSystem, "Previous conversation summary:\n"+summary))
	}

	start := NormalizeCompactedMessages(messages, compactedMessages)
	if start > 1 {
		return base, messages[start:]
	}
	return base, rest
}

func analyzePrepared(base []llms.MessageContent, recent []llms.MessageContent, fullMessages []llms.MessageContent, windowK int, hasSummary bool, summaryTokens int, compactedMessages int) (Info, []llms.MessageContent) {
	windowK = NormalizeWindowK(windowK)
	if len(base) == 0 && len(recent) == 0 {
		return Info{WindowK: windowK, SummaryTokens: summaryTokens, CompactedMessages: compactedMessages, HasSummary: hasSummary}, nil
	}

	fullTokens := EstimateMessagesTokens(fullMessages)
	budget := windowK * 1000
	baseTokens := EstimateMessagesTokens(base)

	chunks := messageChunks(recent)
	selectedChunks := make([][]llms.MessageContent, 0, len(chunks))
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

	selected := make([]llms.MessageContent, 0, len(base)+len(recent))
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

func displayCompactedMessages(messages []llms.MessageContent, compactedMessages int) int {
	count := NormalizeCompactedMessages(messages, compactedMessages)
	if len(messages) > 0 && messages[0].Role == llms.ChatMessageTypeSystem {
		count--
	}
	if count < 0 {
		return 0
	}
	return count
}

func LegacyBuild(messages []llms.MessageContent, windowK int) []llms.MessageContent {
	info, selected := Analyze(messages, windowK)
	if info.Truncated {
		return selected
	}
	return messages
}

func LegacyAnalyze(messages []llms.MessageContent, windowK int) (Info, []llms.MessageContent) {
	windowK = NormalizeWindowK(windowK)
	if len(messages) == 0 {
		return Info{WindowK: windowK}, nil
	}

	fullTokens := EstimateMessagesTokens(messages)
	budget := windowK * 1000
	system, rest := splitSystemMessage(messages)
	systemTokens := EstimateMessagesTokens(system)

	chunks := messageChunks(rest)
	selectedChunks := make([][]llms.MessageContent, 0, len(chunks))
	selectedTokens := systemTokens

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

	selected := make([]llms.MessageContent, 0, len(messages))
	selected = append(selected, system...)
	for i := len(selectedChunks) - 1; i >= 0; i-- {
		selected = append(selected, selectedChunks[i]...)
	}

	return Info{
		WindowK:          windowK,
		FullTokens:       fullTokens,
		SelectedTokens:   selectedTokens,
		FullMessages:     len(messages),
		SelectedMessages: len(selected),
		Truncated:        len(selected) < len(messages) || fullTokens > budget,
	}, selected
}

func EstimateMessagesTokens(messages []llms.MessageContent) int {
	total := 0
	for _, message := range messages {
		total += 4
		for _, part := range message.Parts {
			total += estimatePartTokens(part)
		}
	}
	return total
}

func splitSystemMessage(messages []llms.MessageContent) ([]llms.MessageContent, []llms.MessageContent) {
	if len(messages) == 0 || messages[0].Role != llms.ChatMessageTypeSystem {
		return nil, messages
	}
	return messages[:1], messages[1:]
}

func messageChunks(messages []llms.MessageContent) [][]llms.MessageContent {
	chunks := make([][]llms.MessageContent, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		message := messages[i]
		chunk := []llms.MessageContent{message}
		if message.Role == llms.ChatMessageTypeAI && hasToolCall(message) {
			for i+1 < len(messages) && messages[i+1].Role == llms.ChatMessageTypeTool {
				i++
				chunk = append(chunk, messages[i])
			}
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func hasToolCall(message llms.MessageContent) bool {
	for _, part := range message.Parts {
		if _, ok := part.(llms.ToolCall); ok {
			return true
		}
	}
	return false
}

func estimatePartTokens(part llms.ContentPart) int {
	switch value := part.(type) {
	case llms.TextContent:
		return EstimateTextTokens(value.Text)
	case llms.ToolCall:
		text := value.ID + " " + value.Type
		if value.FunctionCall != nil {
			text += " " + value.FunctionCall.Name + " " + value.FunctionCall.Arguments
		}
		return EstimateTextTokens(text)
	case llms.ToolCallResponse:
		return EstimateTextTokens(value.ToolCallID + " " + value.Name + " " + value.Content)
	default:
		return EstimateTextTokens(fmt.Sprint(value))
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
