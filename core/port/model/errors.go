package model

import "errors"

var (
	// ErrContextWindowExceeded identifies a structured provider context overflow.
	ErrContextWindowExceeded = errors.New("model context window exceeded")
	// ErrIncompleteToolCall prevents incomplete or non-object parameters from
	// being persisted or passed to a tool executor.
	ErrIncompleteToolCall = errors.New("model returned incomplete tool arguments")
)

func IsContextWindowExceeded(err error) bool {
	return errors.Is(err, ErrContextWindowExceeded)
}
