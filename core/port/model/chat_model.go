package model

import "context"

type ChatModelPort interface {
	Generate(ctx context.Context, request GenerateRequest) (ChatResult, error)
}

// ImageInputSupport opts a chat model into the request image format. It describes
// adapter compatibility, not the capabilities of a remote model.
type ImageInputSupport interface {
	SupportsImageInput() bool
}
