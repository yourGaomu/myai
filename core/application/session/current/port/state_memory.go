package port

import "myai/core/session"

type StateMemory interface {
	Current() (*session.Session, error)
	CurrentModelId() string
}
