package runtime

import (
	runtimecommand "myai/core/application/runtime/command"
	runtimeport "myai/core/application/runtime/port"
	runtimeservice "myai/core/application/runtime/service"
)

type InstructionRequest = runtimecommand.InstructionRequest
type SkillPromptProvider = runtimeport.SkillPromptProvider
type AsyncTaskService = runtimeservice.AsyncTaskService
type ModePolicy = runtimeservice.ModePolicy
type RuntimeInstructionBuilder = runtimeservice.RuntimeInstructionBuilder
type SessionPromptProvider = runtimeservice.SessionPromptProvider

const PlanModePrompt = runtimeservice.PlanModePrompt
const RuntimeInstructionPrefix = runtimeservice.RuntimeInstructionPrefix

var NewRuntimeInstructionBuilder = runtimeservice.NewRuntimeInstructionBuilder
var NewSessionPromptProvider = runtimeservice.NewSessionPromptProvider
var InsertRuntimeInstructions = runtimeservice.InsertRuntimeInstructions
