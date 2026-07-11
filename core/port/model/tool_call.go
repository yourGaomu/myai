package model

import (
	domainmessage "myai/core/domain/message"
	tooldef "myai/core/tool/tool"
)

type ToolCall = domainmessage.ToolCall

type ToolPermissionRequest struct {
	Name       string
	Arguments  string
	Permission tooldef.Permission
	Mode       string
}
