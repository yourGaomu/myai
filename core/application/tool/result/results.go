package result

import (
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	tooldef "myai/core/tool/tool"
)

type HookDecision string

const (
	HookDecisionContinue HookDecision = "continue"
	HookDecisionAllow    HookDecision = "allow"
	HookDecisionAsk      HookDecision = "ask"
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
	Calls    []domainmessage.ToolCall
	Messages []domainmessage.Message
	Entries  []domaintool.ExecutionEntry
	Assets   []domaintool.SharedAsset

	imageReads map[string]tooldef.ImageAssetID
}

// ImageReads reports internal image references keyed by their tool call ID.
func (r Execution) ImageReads() map[string]tooldef.ImageAssetID {
	return r.imageReads
}

func (r Execution) WithImageReads(reads map[string]tooldef.ImageAssetID) Execution {
	r.imageReads = reads
	return r
}
