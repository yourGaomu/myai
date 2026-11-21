package model

import "errors"

var (
	// ErrContextWindowExceeded identifies a structured provider context overflow.
	ErrContextWindowExceeded = errors.New("model context window exceeded")
	// ErrIncompleteToolCall prevents incomplete or non-object parameters from
	// being persisted or passed to a tool executor.
	ErrIncompleteToolCall = errors.New("model returned incomplete tool arguments")
	// ErrImageInputUnsupported identifies an adapter without image input mapping.
	ErrImageInputUnsupported = errors.New("this model protocol does not provide image input; select an image-capable Chat Completions model")
	// ErrImageInputRejected identifies a provider rejection of a request with images.
	ErrImageInputRejected = errors.New("the model rejected a request containing images; try a smaller JPEG or PNG, or select a model that supports images")
)

func IsContextWindowExceeded(err error) bool {
	return errors.Is(err, ErrContextWindowExceeded)
}
