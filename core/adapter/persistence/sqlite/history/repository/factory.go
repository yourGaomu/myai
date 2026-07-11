package repository

import historyport "myai/core/port/history"

type Factory struct{}

func (Factory) Open(path string) (historyport.Store, error) {
	return Open(path)
}

func (Factory) DefaultPath(workspace string) (string, error) {
	return DefaultPath(workspace)
}
