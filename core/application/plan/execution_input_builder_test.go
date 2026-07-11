package plan

import (
	"strings"
	"testing"

	agentplan "myai/core/plan"
)

func TestExecutionInputBuilderBuildsFocusedStepInput(t *testing.T) {
	currentPlan := &agentplan.Plan{
		Goal: "Refactor architecture",
		Steps: []agentplan.Step{
			{Order: 1, Status: agentplan.StepStatusDone, Title: "Inspect code"},
			{Order: 2, Status: agentplan.StepStatusRunning, Title: "Extract builder", Description: "Move pure logic"},
		},
	}

	input := ExecutionInputBuilder{}.BuildStepInput(currentPlan, currentPlan.Steps[1], 1, 2)

	for _, expected := range []string{
		"Execute the approved plan step 2/2.",
		"Goal:\nRefactor architecture",
		"Current step:\nExtract builder\nMove pure logic",
		"1. [done] Inspect code",
		"2. [running] Extract builder - Move pure logic",
	} {
		if !strings.Contains(input, expected) {
			t.Fatalf("expected input to contain %q, got:\n%s", expected, input)
		}
	}
}
