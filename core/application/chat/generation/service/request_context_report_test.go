package service

import (
	"testing"

	"myai/core/contextmgr"
	message "myai/core/domain/message"
	modelport "myai/core/port/model"
)

func TestEstimateRequestContextIsAdditiveAndUsesSelectedToolOrigins(t *testing.T) {
	system := message.Text(message.RoleSystem, "You are the assistant.")
	user := message.Text(message.RoleUser, "What is this?")
	summary := message.CompactionSummary("Earlier conversation")
	runtime := message.RuntimeInstruction("Runtime and selected Skill")
	runtime.SkillPromptTokens = 3
	other := message.HookContext("Consider the attached image")
	tools := []modelport.Tool{
		{Type: "function", Function: &modelport.FunctionDefinition{Name: "mcp_lookalike", Parameters: map[string]any{"type": "object"}}},
		(modelport.Tool{Type: "function", Function: &modelport.FunctionDefinition{Name: "remote_tool", Parameters: map[string]any{"type": "object"}}}).WithOrigin(modelport.ToolOriginMCP),
	}
	toolTokens, err := estimateToolTokens(tools)
	if err != nil {
		t.Fatal(err)
	}
	request := modelport.GenerateRequest{Messages: []message.Message{system, summary, runtime, user, other}, Tools: tools}
	budget := contextmgr.NewRequestBudget(13500, toolTokens, 4096)
	const imageTokens = 255
	got := estimateRequestContext("configured-model", budget, request, imageTokens)
	if got.Model != "configured-model" || got.WindowTokens != 13500 || got.OutputReserveTokens != 4096 || got.SafetyReserveTokens != contextmgr.DefaultSafetyTokens {
		t.Fatalf("unexpected context limits: %+v", got)
	}
	if got.SystemTokens != contextmgr.EstimateMessagesTokens([]message.Message{system}) || got.SkillTokens != 3 ||
		got.MessageTokens != contextmgr.EstimateMessagesTokens([]message.Message{user, summary}) ||
		got.MCPToolTokens <= 0 || got.OtherToolTokens <= 0 || got.OtherTokens <= imageTokens {
		t.Fatalf("unexpected categories: %+v", got)
	}
	sum := got.MessageTokens + got.SystemTokens + got.SkillTokens + got.MCPToolTokens + got.OtherToolTokens + got.OtherTokens
	if want := contextmgr.EstimateMessagesTokens(request.Messages) + budget.ToolTokens + imageTokens; sum != want {
		t.Fatalf("estimated input = %d, want %d", sum, want)
	}
	request.Tools = tools[:1]
	budget.ToolTokens, _ = estimateToolTokens(request.Tools)
	if got := estimateRequestContext("configured-model", budget, request, 0); got.MCPToolTokens != 0 || got.OtherToolTokens != budget.ToolTokens {
		t.Fatalf("a local tool with a lookalike name was classified as MCP: %+v", got)
	}
}
