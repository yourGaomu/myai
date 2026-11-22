package model

import (
	domainmessage "myai/core/domain/message"
	tooldef "myai/core/tool/tool"
)

type ToolCall = domainmessage.ToolCall

type ToolPermissionRequest struct {
	ToolCallID string
	Name       string
	Arguments  string
	Permission tooldef.Permission
	Mode       string
	// PermissionIdentity is an internal approval key for the resolved external tool.
	PermissionIdentity string
	// RequireFreshConsent is set by the execution policy, not by the model or approver.
	RequireFreshConsent bool
}
