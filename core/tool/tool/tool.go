package tool

import (
	"context"
	"encoding/json"

	domaintool "myai/core/domain/tool"
)

type ToolOutput = domaintool.ToolOutput

// ImageAssetID identifies a session-authorized image read in an internal execution result.
type ImageAssetID string

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

type TurnBarrier interface {
	IsTurnBarrier() bool
}

type UserInteraction interface {
	RequiresUserInteraction() bool
}

type ExplicitApproval interface {
	RequiresExplicitApproval() bool
}

// SandboxedShell attests that this shell invocation will use an OS sandbox.
// A copied workspace or a nonempty workspace identifier is not sufficient.
type SandboxedShell interface {
	IsSandboxedShell(context.Context) bool
}

type Permission string

const (
	PermissionRead    Permission = "read"
	PermissionWrite   Permission = "write"
	PermissionExecute Permission = "execute"
	PermissionUnknown Permission = "unknown"
)

// NormalizePermission accepts only explicitly declared capabilities. Unknown or
// missing declarations must never acquire read access through normalization.
func NormalizePermission(permission Permission) Permission {
	switch permission {
	case PermissionRead, PermissionWrite, PermissionExecute:
		return permission
	default:
		return PermissionUnknown
	}
}

// PermissionAllowsCeiling distinguishes an unrestricted nil ceiling from an
// explicitly empty ceiling that denies every capability.
func PermissionAllowsCeiling(permission Permission, ceiling []Permission) bool {
	permission = NormalizePermission(permission)
	if permission == PermissionUnknown {
		return false
	}
	if ceiling == nil {
		return true
	}
	for _, allowed := range ceiling {
		if allowed == permission {
			return true
		}
	}
	return false
}

// ClonePermissionCeiling preserves nil versus an explicit empty deny-all set.
func ClonePermissionCeiling(ceiling []Permission) []Permission {
	if ceiling == nil {
		return nil
	}
	return append([]Permission{}, ceiling...)
}
