package tool

import (
	"context"
	"encoding/json"
)

type Tool interface {
	Name() string
	Description() string
	Schema() any
	Permission() Permission
	Call(ctx context.Context, args json.RawMessage) (string, error)
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
