package model

import "context"

type ChatModelPort interface {
	Generate(ctx context.Context, request GenerateRequest) (ChatResult, error)
}
