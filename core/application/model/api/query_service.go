package api

import modelresult "myai/core/application/model/result"

type QueryService interface {
	ListModels() modelresult.ListModels
}
