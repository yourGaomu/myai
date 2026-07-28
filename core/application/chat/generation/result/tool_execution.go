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
}
