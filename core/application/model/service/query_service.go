package service

import (
	modelapi "myai/core/application/model/api"
	appmodelport "myai/core/application/model/port"
	modelresult "myai/core/application/model/result"
)

type QueryService struct {
	Catalog appmodelport.ModelCatalog
}

var _ modelapi.QueryService = QueryService{}

func (s QueryService) ListModels() modelresult.ListModels {
	if s.Catalog == nil {
		return modelresult.ListModels{}
	}
	models := s.Catalog.ListModels()
	return modelresult.ListModels{Models: append(models[:0:0], models...)}
}
