package result

import (
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
)

type ToolExecution struct {
	Calls    []domainmessage.ToolCall
	Messages []domainmessage.Message
	Entries  []domaintool.ExecutionEntry
	Assets   []domaintool.SharedAsset

	imageReads map[string]string
}

// ImageReads reports trusted image asset IDs keyed by the exact tool call ID.
// They are private to the generation request and are not part of tool messages.
func (r ToolExecution) ImageReads() map[string]string {
	return r.imageReads
}

func (r ToolExecution) WithImageReads(reads map[string]string) ToolExecution {
	r.imageReads = reads
	return r
}
