package service

import asyncport "myai/core/port/async"

type AsyncTaskService struct {
	Executor asyncport.Executor
	Fallback func(task func())
}

func (s AsyncTaskService) Submit(task func()) {
	if task == nil {
		return
	}
	if s.Executor == nil {
		s.runFallback(task)
		return
	}
	if err := s.Executor.Submit(task); err != nil {
		s.runFallback(task)
	}
}

func (s AsyncTaskService) runFallback(task func()) {
	if s.Fallback != nil {
		s.Fallback(task)
		return
	}
	go task()
}
