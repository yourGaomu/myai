package history

import historyrepository "myai/core/adapter/persistence/sqlite/history/repository"

type Store = historyrepository.Store
type Factory = historyrepository.Factory

var Open = historyrepository.Open
var DefaultPath = historyrepository.DefaultPath
