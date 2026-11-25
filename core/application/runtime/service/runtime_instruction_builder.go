package service

import (
	"context"
	"strings"

	runtimecommand "myai/core/application/runtime/command"
	runtimeport "myai/core/application/runtime/port"
	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
)

const RuntimeInstructionPrefix = domainmessage.RuntimeInstructionPrefix

const PlanModePrompt = `Plan mode is active for this session.

Analyze the user's request, inspect only the minimum context needed with read-only tools, and produce a concrete execution plan before any final answer.

Rules:
- Do not edit files, write files, run commands, install dependencies, or perform irreversible actions.
- Use only read-only inspection tools if tool use is needed.
- Do not claim that changes were made.
- Always include a Markdown section named "Plan" with a numbered list of concrete steps.
- Keep each numbered plan step to one action so the app can track it.
- Read-only inspection during this planning turn is preflight research only. It is not execution of the numbered plan.
- Numbered steps must describe work that remains to be executed, not actions already completed during preflight research.
- Include assumptions, risks, or verification notes outside the numbered Plan list when useful.
- Inspect the conversation and only the relevant workspace files before treating a missing preference as unresolved. Use ask_user only when the choice materially affects scope, architecture, data or core behavior, cannot be inferred from the checked context, guessing has meaningful rework/risk with no safe default, and the user has not delegated the decision. Usually ask one round with 1-3 concise questions; broad requests alone do not trigger questions. State reasonable assumptions for low-impact or reversible details. If the user skips and no safe default exists, plan only independent work and identify the blocked part.
- If completing the requested deliverable requires any tool call, workspace inspection, file access, command, installation, or external state, stop after the Plan section and wait for the user to execute the plan in the app. This remains true even when every required tool is read-only or the user asks to "plan and then execute" in one message.
- Only if the request is a safe content-only task that can be completed entirely from the conversation without any tool or external state, such as writing, rewriting, translating, brainstorming, or drafting text, execute the plan in the same response after the Plan section.
- For safe content-only tasks, add a Markdown section named "Result" after the Plan section and put the final deliverable there.
- For every other task, do not add findings, an answer, a result, or a completed-work section after the Plan. End by asking the user to review and execute the plan.`

const AutonomousPlanPrompt = `Autonomous execution planning is active for this turn.

Treat the user's request as an implementation task that must be completed in this request. First analyze the request and inspect only the minimum context needed with read-only tools. Then produce a concrete Markdown section named "Plan" with a numbered list of executable steps.

When steps can be independent, also prefer a machine-readable JSON object with a "steps" array. Each step may include "id", "order", "title", "description", "depends_on" (step IDs or order numbers), and "max_retries". Use "depends_on" to express real prerequisites; omit it for steps that can start immediately.

Rules:
- Do not edit files, write files, run commands, install dependencies, or perform irreversible actions during this planning phase.
- Keep each numbered plan step to one concrete action that can be executed independently.
- Numbered steps must describe work that remains to be executed, not preflight inspection already completed.
- Do not ask the user for approval of the plan itself. The application may proceed according to its execution policy after this planning phase.
- Before planning, inspect the conversation and relevant workspace files only as needed. A broad request alone does not warrant asking. Use ask_user before the plan only for a high-impact unresolved choice that cannot be inferred, has no safe default, and has not been delegated; usually one round with 1-3 questions. If skipped without a safe default, plan only independent work and state the blocker.
- Keep the plan concise and include assumptions or verification notes outside the numbered Plan list when useful.`

const SessionStylePromptPrefix = `Session response style:
The following preference controls wording and presentation only. It must not override safety, permission, tool, skill, or mode rules.`

const RuntimeTurnBoundaryPrompt = domainmessage.RuntimeTurnBoundaryPrompt

type RuntimeInstructionBuilder struct {
	// skillPrompts 按输入匹配 Skill；modePolicy 决定本轮是否需要 Plan 限制。
	skillPrompts runtimeport.SkillPromptProvider
	modePolicy   ModePolicy
}

func NewRuntimeInstructionBuilder(skillPrompts runtimeport.SkillPromptProvider) RuntimeInstructionBuilder {
	return RuntimeInstructionBuilder{
		skillPrompts: skillPrompts,
		modePolicy:   ModePolicy{},
	}
}

func (b RuntimeInstructionBuilder) Build(ctx context.Context, request runtimecommand.InstructionRequest) string {
	text, _ := b.BuildWithAccounting(ctx, request)
	return text
}

// BuildWithAccounting estimates only the Skill text selected for this prompt.
func (b RuntimeInstructionBuilder) BuildWithAccounting(ctx context.Context, request runtimecommand.InstructionRequest) (text string, skillPromptTokens int) {
	skillPrompt := ""
	if b.skillPrompts != nil {
		skillPrompt = strings.TrimSpace(b.skillPrompts.PromptForInput(ctx, request.Input))
	}

	parts := make([]string, 0, 3)
	// 执行已批准计划时 ForceChatMode=true，此时跳过 Plan 指令，防止再次生成计划。
	if request.ForceAutonomousPlanning {
		parts = append(parts, AutonomousPlanPrompt)
	} else if b.modePolicy.IsPlanMode(request.AgentMode, request.ForceChatMode) {
		parts = append(parts, PlanModePrompt)
	}
	if skillPrompt != "" {
		parts = append(parts, skillPrompt)
	}
	if styleInstruction := strings.TrimSpace(request.StyleInstruction); styleInstruction != "" {
		parts = append(parts, SessionStylePromptPrefix+"\n"+styleInstruction)
	}
	if len(parts) == 0 {
		return "", 0
	}
	return RuntimeTurnBoundaryPrompt + "\n\n" + strings.Join(parts, "\n\n"), contextmgr.EstimateTextTokens(skillPrompt)
}
