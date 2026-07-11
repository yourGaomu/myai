package uuidadapter

import "github.com/google/uuid"

type Generator struct{}

func (Generator) NewID() string {
	return uuid.NewString()
}

func (Generator) NewRequestID() string {
	return (Generator{}).NewID()
}
