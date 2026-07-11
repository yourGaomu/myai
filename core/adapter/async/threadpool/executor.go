package threadpool

import (
	"errors"
)

type Executor struct {
	Pool *Pool
}

func (e Executor) Submit(task func()) error {
	if e.Pool == nil {
		return errors.New("thread pool is nil")
	}
	return e.Pool.Submit(task)
}
