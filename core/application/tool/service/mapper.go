package service

import (
	toolcommand "myai/core/application/tool/command"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
)

func ToolCallEntry(command toolcommand.ToolCallEntry) domaintool.ExecutionEntry {
	return domaintool.ExecutionEntry{Kind: domaintool.ExecutionEntryToolCall, SessionID: command.SessionID, ToolCallID: command.Call.ID, ToolName: command.Call.Name, Arguments: command.Call.Arguments, CreatedAt: command.CreatedAt}
}

func ToolResultEntry(command toolcommand.ToolResultEntry) domaintool.ExecutionEntry {
	return domaintool.ExecutionEntry{Kind: domaintool.ExecutionEntryToolResult, SessionID: command.SessionID, ToolCallID: command.Call.ID, ToolName: command.Call.Name, Arguments: command.Call.Arguments, Content: command.Result, Error: command.ToolError, CreatedAt: command.CreatedAt}
}

func ToolResultMessage(call domainmessage.ToolCall, result string) domainmessage.Message {
	return domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: result})
}
