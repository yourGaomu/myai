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
	output := command.Output.Normalized()
	return domaintool.ExecutionEntry{Kind: domaintool.ExecutionEntryToolResult, SessionID: command.SessionID, ToolCallID: command.Call.ID, ToolName: command.Call.Name, Arguments: command.Call.Arguments, Content: output.Content, Error: output.ErrorMessage, Status: output.Status, ErrorCode: output.ErrorCode, Truncated: output.Truncated, CreatedAt: command.CreatedAt}
}

func ToolResultMessage(call domainmessage.ToolCall, output domaintool.ToolOutput) domainmessage.Message {
	output = output.Normalized()
	return domainmessage.ToolResultMessage(domainmessage.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: output.Content, Status: output.Status, ErrorCode: output.ErrorCode, ErrorMessage: output.ErrorMessage, Truncated: output.Truncated})
}
