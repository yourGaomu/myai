package service

import (
	"errors"
	"fmt"

	asyncport "myai/core/port/async"
)

type AsyncTaskService struct {
	Executor asyncport.Executor
}

func (s AsyncTaskService) Submit(task func()) error {
	if task == nil {
		return nil
	}
	if s.Executor == nil {
		return asyncport.ErrExecutorUnavailable
	}
	if err := s.Executor.Submit(task); err != nil {
		if errors.Is(err, asyncport.ErrQueueFull) {
			return runInCaller(task)
		}
		return err
	}
	return nil
}

func runInCaller(task func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("async caller-runs task panicked: %v", recovered)
		}
	}()
	task()
	return nil
}
