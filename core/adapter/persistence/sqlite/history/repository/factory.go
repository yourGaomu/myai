package repository

import historyport "myai/core/port/history"

type Factory struct {
	// BaseDir is the history data directory. Empty preserves the user config default.
	BaseDir string
}

func (Factory) Open(path string) (historyport.Store, error) {
	return Open(path)
}

func (f Factory) DefaultPath(workspace string) (string, error) {
	return defaultPath(workspace, f.BaseDir)
}
