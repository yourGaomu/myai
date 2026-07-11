package result

import domainmodel "myai/core/domain/model"

type Bootstrap struct {
	Configs        []domainmodel.Config
	DefaultModelID string
}
