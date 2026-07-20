package tool

import (
	"context"
	"encoding/json"

	domaintool "myai/core/domain/tool"
)

type ToolOutput = domaintool.ToolOutput

func SuccessOutput(content string) ToolOutput {
	return domaintool.SuccessOutput(content)
}

func FailedOutput(status domaintool.ResultStatus, errorCode string, message string) ToolOutput {
	return domaintool.FailedOutput(status, errorCode, message)
}

type Tool interface {
	Name() string
	Description() string
	Schema() any
	Permission() Permission
	Call(ctx context.Context, args json.RawMessage) (ToolOutput, error)
}

type Permission string

const (
	PermissionRead    Permission = "read"
	PermissionWrite   Permission = "write"
	PermissionExecute Permission = "execute"
)

func NormalizePermission(permission Permission) Permission {
	switch permission {
	case PermissionWrite, PermissionExecute:
		return permission
	default:
		return PermissionRead
	}
}
