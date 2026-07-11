package toolapp

import (
	toolapi "myai/core/application/tool/api"
	toolcommand "myai/core/application/tool/command"
	toolport "myai/core/application/tool/port"
	toolresult "myai/core/application/tool/result"
	toolservice "myai/core/application/tool/service"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
)

type AssetExtractionCommand = toolcommand.AssetExtraction
type ExecutionCallbacks = toolcommand.ExecutionCallbacks
type ExecutionCommand = toolcommand.Execution
type HookEvent = toolcommand.HookEvent
type PermissionAskFunc = toolcommand.PermissionAskFunc
type PermissionCommand = toolcommand.Permission
type PermissionRequest = toolcommand.PermissionRequest
type ToolCallEntryCommand = toolcommand.ToolCallEntry
type ToolResultEntryCommand = toolcommand.ToolResultEntry

type ExecutionResult = toolresult.Execution
type HookDecision = toolresult.HookDecision
type HookResult = toolresult.Hook
type PermissionDecision = toolresult.PermissionDecision

type AssetExtractor = toolport.AssetExtractor
type HookBridge = toolport.HookBridge
type LLMToolCatalog = toolport.LLMToolCatalog
type Registry = toolport.Registry
type ToolModePolicy = toolport.ToolModePolicy

type ExecutionService = toolservice.ExecutionService
type PermissionService = toolservice.PermissionService
type SelectionService = toolservice.SelectionService
type PermissionServiceAPI = toolapi.PermissionService

const (
	HookDecisionContinue = toolresult.HookDecisionContinue
	HookDecisionAllow    = toolresult.HookDecisionAllow
	HookDecisionDeny     = toolresult.HookDecisionDeny
)

func ToolCallEntry(command ToolCallEntryCommand) domaintool.ExecutionEntry {
	return toolservice.ToolCallEntry(command)
}

func ToolResultEntry(command ToolResultEntryCommand) domaintool.ExecutionEntry {
	return toolservice.ToolResultEntry(command)
}

func ToolResultMessage(call domainmessage.ToolCall, result string) domainmessage.Message {
	return toolservice.ToolResultMessage(call, result)
}
