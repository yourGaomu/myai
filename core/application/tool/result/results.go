package result

import (
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
)

type HookDecision string

const (
	HookDecisionContinue HookDecision = "continue"
	HookDecisionAllow    HookDecision = "allow"
	HookDecisionDeny     HookDecision = "deny"
)

type Hook struct {
	Decision  HookDecision
	Arguments string
	Message   string
}

type PermissionDecision struct {
	Message string
	Allowed bool
}

type Execution struct {
	Messages []domainmessage.Message
	Entries  []domaintool.ExecutionEntry
	Assets   []domaintool.SharedAsset
}
