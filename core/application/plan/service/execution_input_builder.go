package service

import (
	"fmt"
	"strings"

	agentplan "myai/core/plan"
)

type ExecutionInputBuilder struct{}

func (ExecutionInputBuilder) BuildStepInput(currentPlan *agentplan.Plan, step agentplan.Step, index int, total int) string {
	var builder strings.Builder
	builder.WriteString("Execute the approved plan step ")
	builder.WriteString(fmt.Sprintf("%d/%d", index+1, total))
	builder.WriteString(".\n\n")
	if currentPlan != nil && strings.TrimSpace(currentPlan.Goal) != "" {
		builder.WriteString("Goal:\n")
		builder.WriteString(currentPlan.Goal)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Current step:\n")
	builder.WriteString(step.Title)
	if strings.TrimSpace(step.Description) != "" {
		builder.WriteString("\n")
		builder.WriteString(step.Description)
	}
	builder.WriteString("\n\nFull plan:\n")
	if currentPlan != nil {
		for _, item := range currentPlan.Steps {
			builder.WriteString(fmt.Sprintf("%d. [%s] %s", item.Order, item.Status, item.Title))
			if strings.TrimSpace(item.Description) != "" {
				builder.WriteString(" - ")
				builder.WriteString(item.Description)
			}
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nFocus on the current step. Use tools when needed, report what changed, and stop after this step is complete.")
	return builder.String()
}
