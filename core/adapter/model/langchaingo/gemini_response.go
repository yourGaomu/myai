package langchaingo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tmc/langchaingo/llms"

	corellm "myai/core/llm"
)

type geminiUsage struct {
	Prompt    wireTokenCount `json:"promptTokenCount"`
	Output    wireTokenCount `json:"candidatesTokenCount"`
	Total     wireTokenCount `json:"totalTokenCount"`
	Cached    wireTokenCount `json:"cachedContentTokenCount"`
	Reasoning wireTokenCount `json:"thoughtsTokenCount"`
}

func (u *geminiUsage) merge(next geminiUsage) {
	u.Prompt.update(next.Prompt)
	u.Output.update(next.Output)
	u.Total.update(next.Total)
	u.Cached.update(next.Cached)
	u.Reasoning.update(next.Reasoning)
}

func (u geminiUsage) tokenUsage() corellm.TokenUsage {
	total := u.Total.value
	if !u.Total.present && u.Prompt.present && u.Output.present {
		total = u.Prompt.value + u.Output.value + u.Reasoning.value
	}
	return corellm.TokenUsage{
		Available:    u.Prompt.present || u.Output.present || u.Total.present || u.Reasoning.present,
		PromptTokens: u.Prompt.value, CompletionTokens: u.Output.value, TotalTokens: total,
		PromptCachedTokens: u.Cached.value, PromptCacheReported: u.Cached.present, ReasoningTokens: u.Reasoning.value,
	}
}

type geminiFrame struct {
	Candidates []struct {
		FinishReason json.RawMessage `json:"finishReason"`
		Content      struct {
			Parts []struct {
				Text         *string `json:"text"`
				Thought      bool    `json:"thought"`
				FunctionCall *struct {
					Name string          `json:"name"`
					Args json.RawMessage `json:"args"`
				} `json:"functionCall"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason json.RawMessage `json:"blockReason"`
	} `json:"promptFeedback"`
	Usage geminiUsage     `json:"usageMetadata"`
	Error json.RawMessage `json:"error"`
}

func readGeminiResponse(ctx context.Context, body io.Reader, streaming bool, onText func(context.Context, []byte) error) (*llms.ContentResponse, error) {
	decoder := json.NewDecoder(body)
	if streaming {
		start, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("open Gemini stream: %w", err)
		}
		corellm.ObserveResponseChunk(ctx)
		if start != json.Delim('[') {
			return nil, errors.New("Gemini stream is not a JSON array")
		}
	}

	var (
		text          strings.Builder
		choice        llms.ContentChoice
		usage         geminiUsage
		candidateSeen bool
	)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if streaming && !decoder.More() {
			break
		}
		var frame geminiFrame
		if err := decoder.Decode(&frame); err != nil {
			return nil, fmt.Errorf("decode Gemini response: %w", err)
		}
		if len(frame.Error) != 0 && string(frame.Error) != "null" {
			return nil, errors.New("Gemini returned a response error")
		}
		if block := geminiEnum(frame.PromptFeedback.BlockReason); block != "" && block != "BLOCK_REASON_UNSPECIFIED" {
			return nil, errors.New("Gemini blocked the prompt")
		}
		usage.merge(frame.Usage)
		if len(frame.Candidates) > 1 {
			return nil, errors.New("Gemini returned more than one candidate")
		}
		for _, candidate := range frame.Candidates {
			candidateSeen = true
			if reason := geminiEnum(candidate.FinishReason); reason != "" && reason != "FINISH_REASON_UNSPECIFIED" {
				var stopReason string
				switch reason {
				case "1", "STOP":
					stopReason = "FinishReasonStop"
				case "2", "MAX_TOKENS":
					stopReason = "FinishReasonMaxTokens"
				case "3", "4", "7", "8", "9", "11", "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY":
					return nil, errors.New("Gemini blocked the response")
				case "5", "OTHER":
					stopReason = "FinishReasonOther"
				default:
					stopReason = reason
				}
				if choice.StopReason != "" && choice.StopReason != stopReason {
					return nil, errors.New("Gemini returned conflicting finish reasons")
				}
				choice.StopReason = stopReason
			}
			for _, part := range candidate.Content.Parts {
				switch {
				case part.FunctionCall != nil:
					call := part.FunctionCall
					args := call.Args
					if len(args) == 0 || string(args) == "null" {
						args = json.RawMessage("{}")
					}
					choice.ToolCalls = append(choice.ToolCalls, llms.ToolCall{FunctionCall: &llms.FunctionCall{Name: call.Name, Arguments: string(args)}})
				case part.Text != nil:
					if part.Thought || *part.Text == "" {
						continue
					}
					if onText != nil {
						if err := onText(ctx, []byte(*part.Text)); err != nil {
							return nil, err
						}
					}
					text.WriteString(*part.Text)
				default:
					return nil, errors.New("Gemini returned an unsupported content part")
				}
			}
		}
		if !streaming {
			break
		}
	}
	if streaming {
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return nil, corellm.ErrIncompleteResponse
		}
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after Gemini response")
	}
	if !candidateSeen {
		return nil, corellm.ErrEmptyResponse
	}
	if len(choice.ToolCalls) != 0 && choice.StopReason != "FinishReasonStop" {
		return nil, corellm.ErrIncompleteToolCall
	}
	if streaming && choice.StopReason == "" {
		return nil, corellm.ErrIncompleteResponse
	}
	choice.Content = text.String()
	corellm.ObserveTokenUsage(ctx, usage.tokenUsage())
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{&choice}}, nil
}

// Gemini requests numeric enums but some responses still use their names.
func geminiEnum(raw json.RawMessage) string {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name
	}
	if value, err := strconv.Atoi(string(raw)); err == nil && value > 0 {
		return string(raw)
	}
	return ""
}
