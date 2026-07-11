package command

import domainmodel "myai/core/domain/model"

type Bootstrap struct {
	Seed            domainmodel.Config
	FallbackModelID string
}
