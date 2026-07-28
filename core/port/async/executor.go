package async

import "errors"

var (
	ErrExecutorUnavailable = errors.New("async executor is unavailable")
	ErrExecutorClosed      = errors.New("async executor is closed")
	ErrQueueFull           = errors.New("async task queue is full")
)

type Executor interface {
	Submit(task func()) error
}
