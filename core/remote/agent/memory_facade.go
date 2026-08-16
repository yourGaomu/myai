package agent

import memorycatalogapi "myai/core/application/memory/catalog/api"
import memorydreamapi "myai/core/application/memory/dream/api"
import memoryextractionapi "myai/core/application/memory/extraction/api"

type MemoryFacade interface {
	memorycatalogapi.Service
}

type MemoryExtractionFacade interface {
	memoryextractionapi.Service
}

type MemoryDreamFacade interface {
	memorydreamapi.Service
}
