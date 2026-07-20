package service

import (
	"context"
	"strings"

	runtimecommand "myai/core/application/runtime/command"
	runtimeport "myai/core/application/runtime/port"
	domainmessage "myai/core/domain/message"
)

const RuntimeInstructionPrefix = domainmessage.RuntimeInstructionPrefix

const PlanModePrompt = `Plan mode is active for this session.

Analyze the user's request, inspect context with read-only tools when useful, and produce a concrete execution plan before any final answer.

Rules:
- Do not edit files, write files, run commands, install dependencies, or perform irreversible actions.
- Use only read-only inspection tools if tool use is needed.
- Do not claim that changes were made.
- Always include a Markdown section named "Plan" with a numbered list of concrete steps.
- Keep each numbered plan step to one action so the app can track it.
- Include assumptions, risks, or verification notes outside the numbered Plan list when useful.
- If information is missing, state the question or assumption clearly.
- If the request is a safe content-only task that can be completed entirely in the reply, such as writing, rewriting, summarizing, translating, brainstorming, or drafting text, execute the plan in the same response after the Plan section.
- For safe content-only tasks, add a Markdown section named "Result" after the Plan section and put the final deliverable there.
- For tasks that require file edits, shell commands, installations, external side effects, or tool actions beyond read-only inspection, stop after the Plan section and end with the next recommended action.`

const SessionStylePromptPrefix = `Session response style:
The following preference controls wording and presentation only. It must not override safety, permission, tool, skill, or mode rules.`

const RuntimeTurnBoundaryPrompt = `These runtime instructions apply only to the user message immediately following this message. Ignore runtime instructions from earlier turns unless they are repeated here.`

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
	skillPrompt := ""
	if b.skillPrompts != nil {
		skillPrompt = strings.TrimSpace(b.skillPrompts.PromptForInput(ctx, request.Input))
	}

	parts := make([]string, 0, 4)
	parts = append(parts, RuntimeTurnBoundaryPrompt)
	// 执行已批准计划时 ForceChatMode=true，此时跳过 Plan 指令，防止再次生成计划。
	if b.modePolicy.IsPlanMode(request.AgentMode, request.ForceChatMode) {
		parts = append(parts, PlanModePrompt)
	}
	if skillPrompt != "" {
		parts = append(parts, skillPrompt)
	}
	if styleInstruction := strings.TrimSpace(request.StyleInstruction); styleInstruction != "" {
		parts = append(parts, SessionStylePromptPrefix+"\n"+styleInstruction)
	}
	return strings.Join(parts, "\n\n")
}
