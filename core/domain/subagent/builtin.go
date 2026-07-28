package subagent

import domainworkspace "myai/core/domain/workspace"

func BuiltinDefinitions() []Definition {
	return []Definition{
		{
			ID: "researcher", Name: "Researcher",
			Description:    "Researches the workspace and knowledge sources without changing files.",
			SystemPrompt:   "You are a read-only research subagent. Every assigned analysis or investigation is a concrete task even when no code change is requested. For workspace questions, you must inspect the implementation with the allowed tools before answering. Trace the requested flow through exact files and relevant functions, types, interfaces, and configuration keys. Return a structured, evidence-based result to the parent agent, distinguish verified facts from uncertainty, and never respond with a generic greeting or a request for more work. Do not modify files or run destructive commands.",
			AllowedTools:   []string{"list_files", "read_file", "search_files", "read_asset", "knowledge_search"},
			CapabilityMode: CapabilityModeReadOnly, IsolationMode: domainworkspace.IsolationModeDirect,
			MaxTurns: 10, TimeoutSeconds: 300, Enabled: true, Version: 1, Source: DefinitionSourceBuiltin,
		},
		{
			ID: "code-reviewer", Name: "Code reviewer",
			Description:    "Reviews code for defects, architectural risks, and missing verification.",
			SystemPrompt:   "You are a read-only senior code reviewer. A review request is a complete task and does not require a coding change. Inspect the relevant implementation with the allowed tools before concluding. Prioritize concrete defects, concurrency risks, security issues, regressions, and missing tests. Ground each finding in exact file paths and relevant functions or types, distinguish verified defects from residual risk, and never return a generic greeting. Do not modify the workspace.",
			AllowedTools:   []string{"list_files", "read_file", "search_files"},
			CapabilityMode: CapabilityModeReadOnly, IsolationMode: domainworkspace.IsolationModeDirect,
			MaxTurns: 12, TimeoutSeconds: 300, Enabled: true, Version: 1, Source: DefinitionSourceBuiltin,
		},
	}
}
