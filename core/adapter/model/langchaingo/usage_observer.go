package langchaingo

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	corellm "myai/core/llm"
)

// The observer forwards all bytes unchanged and looks only at usage. Streaming
// frames use the SDK's data-line format. For nonstream JSON, scan past large
// message bodies without buffering them; retain only the top-level usage object.
type usageObserver struct {
	ctx       context.Context
	protocol  usageProtocol
	streaming bool
	line      []byte
	dropping  bool
	done      bool
	object    jsonUsageObject
	usage     corellm.TokenUsage
	prompt    wireTokenCount
	output    wireTokenCount
	total     wireTokenCount
	creation  wireTokenCount
	cached    wireTokenCount
	reasoning wireTokenCount
}

type usageProtocol uint8

const (
	noUsageObservation usageProtocol = iota
	chatCompletionsUsage
	anthropicMessagesUsage
	maxUsageBytes = 64 * 1024
)

// A pointer to an int would distinguish absence from zero, but a malformed
// optional counter would then discard the other valid counters in the frame.
type wireTokenCount struct {
	value   int
	present bool
}

func (count *wireTokenCount) UnmarshalJSON(raw []byte) error {
	value, err := strconv.Atoi(string(raw))
	*count = wireTokenCount{value: value, present: err == nil && value >= 0}
	return nil
}

func (count *wireTokenCount) update(candidates ...wireTokenCount) {
	for _, candidate := range candidates {
		if candidate.present {
			*count = candidate
			return
		}
	}
}

func (o *usageObserver) read(data []byte, eof bool) {
	if !o.streaming {
		if usage := o.object.read(data); usage != nil {
			o.observe(usage)
		}
		return
	}
	for len(data) > 0 && !o.done {
		line, remaining, ended := bytes.Cut(data, []byte{'\n'})
		if !o.dropping {
			if len(o.line)+len(line) > maxUsageBytes {
				o.line = o.line[:0]
				o.dropping = true
			} else if len(o.line) == 0 && ended {
				o.observeLine(line)
			} else {
				o.line = append(o.line, line...)
				if ended {
					o.observeLine(o.line)
				}
			}
		}
		if ended {
			o.line = o.line[:0]
			o.dropping = false
		}
		data = remaining
	}
	if eof && !o.done && !o.dropping && len(o.line) != 0 {
		o.observeLine(o.line)
	}
}

func (o *usageObserver) observeLine(line []byte) {
	data, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok {
		return
	}
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("[DONE]")) {
		o.done = true
		return
	}
	if o.protocol == chatCompletionsUsage && !bytes.Contains(data, []byte(`"usage"`)) {
		return
	}
	var event struct {
		Type    string          `json:"type"`
		Usage   json.RawMessage `json:"usage"`
		Message struct {
			Usage json.RawMessage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	if o.protocol == anthropicMessagesUsage {
		switch event.Type {
		case "message_start":
			o.observe(event.Message.Usage)
		case "message_delta":
			o.observe(event.Usage)
		}
		return
	}
	o.observe(event.Usage)
}

func (o *usageObserver) observe(raw []byte) {
	if len(raw) == 0 {
		return
	}
	if o.protocol == anthropicMessagesUsage {
		var usage struct {
			Input    wireTokenCount `json:"input_tokens"`
			Output   wireTokenCount `json:"output_tokens"`
			Creation wireTokenCount `json:"cache_creation_input_tokens"`
			Read     wireTokenCount `json:"cache_read_input_tokens"`
		}
		if json.Unmarshal(raw, &usage) != nil {
			return
		}
		o.prompt.update(usage.Input)
		o.output.update(usage.Output)
		o.creation.update(usage.Creation)
		o.cached.update(usage.Read)
	} else {
		var usage struct {
			Prompt         wireTokenCount `json:"prompt_tokens"`
			Output         wireTokenCount `json:"completion_tokens"`
			Total          wireTokenCount `json:"total_tokens"`
			DeepSeekHit    wireTokenCount `json:"prompt_cache_hit_tokens"`
			PromptCached   wireTokenCount `json:"prompt_cached_tokens"`
			CachedPrompt   wireTokenCount `json:"cached_prompt_tokens"`
			Cached         wireTokenCount `json:"cached_tokens"`
			CacheReadInput wireTokenCount `json:"cache_read_input_tokens"`
			CacheRead      wireTokenCount `json:"cache_read_tokens"`
			Reasoning      wireTokenCount `json:"reasoning_tokens"`
			PromptDetails  struct {
				Cached wireTokenCount `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			InputDetails struct {
				Cached wireTokenCount `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			CompletionDetails struct {
				Reasoning wireTokenCount `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		}
		if json.Unmarshal(raw, &usage) != nil {
			return
		}
		o.prompt.update(usage.Prompt)
		o.output.update(usage.Output)
		o.total.update(usage.Total)
		o.cached.update(usage.PromptDetails.Cached, usage.InputDetails.Cached, usage.DeepSeekHit, usage.PromptCached, usage.CachedPrompt, usage.Cached, usage.CacheReadInput, usage.CacheRead)
		o.reasoning.update(usage.CompletionDetails.Reasoning, usage.Reasoning)
	}
	prompt := o.prompt.value
	promptPresent := o.prompt.present
	if o.protocol == anthropicMessagesUsage {
		// AN's input_tokens is uncached input only. All three components
		// belong to the input total, but only cache reads are hits.
		prompt += o.creation.value + o.cached.value
		promptPresent = promptPresent || o.creation.present || o.cached.present
	}
	total := o.total.value
	if !o.total.present && promptPresent && o.output.present {
		total = prompt + o.output.value
	}
	usage := corellm.TokenUsage{
		Available:    promptPresent || o.output.present || o.total.present,
		PromptTokens: prompt, CompletionTokens: o.output.value, TotalTokens: total,
		PromptCachedTokens: o.cached.value, PromptCacheReported: o.cached.present,
		ReasoningTokens: o.reasoning.value,
	}
	if usage != o.usage {
		o.usage = usage
		corellm.ObserveTokenUsage(o.ctx, usage)
	}
}

// jsonUsageObject locates the first top-level usage object in a JSON response.
// It tracks string escapes and nesting even in skipped values, so large content,
// nested usage keys, and usage-like strings in answers are not retained/parsed.
// The SDK still owns validation and consumption of the full response.
type jsonUsageObject struct {
	depth               int
	inString, escaped   bool
	wantKey, readingKey bool
	usageKey, wantValue bool
	capturing, done     bool
	key                 [len(`"\u0075\u0073\u0061\u0067\u0065"`)]byte
	keyLen              int
	value               []byte
}

func (o *jsonUsageObject) read(data []byte) []byte {
	if o.done {
		return nil
	}
	for _, c := range data {
		if o.capturing {
			if len(o.value) == maxUsageBytes {
				o.done = true
				return nil
			}
			o.value = append(o.value, c)
		}
		if o.readingKey {
			if o.keyLen < len(o.key) {
				o.key[o.keyLen] = c
			}
			o.keyLen++
		}
		if o.inString {
			switch {
			case o.escaped:
				o.escaped = false
			case c == '\\':
				o.escaped = true
			case c == '"':
				o.inString = false
				if o.readingKey {
					o.readingKey = false
					var key string
					o.usageKey = o.keyLen <= len(o.key) && json.Unmarshal(o.key[:o.keyLen], &key) == nil && key == "usage"
				}
			}
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		if o.wantValue {
			o.wantValue = false
			if c == '{' {
				o.capturing = true
				o.value = append(o.value, c)
			}
		}
		switch c {
		case '"':
			o.inString = true
			if o.depth == 1 && o.wantKey {
				o.readingKey, o.wantKey = true, false
				o.key[0], o.keyLen = c, 1
			}
		case ':':
			if o.depth == 1 {
				o.wantValue = o.usageKey
			}
		case '{', '[':
			if o.depth == 0 {
				if c != '{' {
					o.done = true
					return nil
				}
				o.wantKey = true
			}
			o.depth++
		case '}', ']':
			o.depth--
			if o.capturing && o.depth == 1 {
				o.done = true
				return o.value
			}
			if o.depth <= 0 {
				o.done = true
				return nil
			}
		case ',':
			if o.depth == 1 {
				o.usageKey, o.wantKey = false, true
			}
		}
	}
	return nil
}
