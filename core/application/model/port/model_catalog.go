package port

import modelport "myai/core/port/model"

type ModelCatalog interface {
	ListModels() []modelport.ModelInfo
}
