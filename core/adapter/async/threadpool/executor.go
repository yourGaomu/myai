package threadpool

import (
	asyncport "myai/core/port/async"
)

type Executor struct {
	Pool *Pool
}

func (e Executor) Submit(task func()) error {
	if e.Pool == nil {
		return asyncport.ErrExecutorUnavailable
	}
	return e.Pool.Submit(task)
}
