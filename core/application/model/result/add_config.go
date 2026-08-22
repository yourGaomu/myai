package result

import domainmodel "myai/core/domain/model"

type AddConfig struct {
	Config domainmodel.Config
}

type ConfigMutation struct {
	Config    domainmodel.Config
	DeletedID string
	Message   string
}
