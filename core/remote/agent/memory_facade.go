package agent

import memorycatalogapi "myai/core/application/memory/catalog/api"

type MemoryFacade interface {
	memorycatalogapi.Service
}
