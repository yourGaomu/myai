package plan

import (
	plancommand "myai/core/application/plan/command"
	planservice "myai/core/application/plan/service"
)

type SaveStateCommand = plancommand.SaveState
type CaptureService = planservice.CaptureService
type ExecutionInputBuilder = planservice.ExecutionInputBuilder
type ResponseCombiner = planservice.ResponseCombiner
type StatePersistenceService = planservice.StatePersistenceService
type StateService = planservice.StateService
