package model

import (
	"context"

	domainmodel "myai/core/domain/model"
)

type ConfigWriter interface {
	SaveConfig(ctx context.Context, config domainmodel.Config) error
}

type ConfigReader interface {
	ListConfigs(ctx context.Context) ([]domainmodel.Config, error)
}

type ConfigRepository interface {
	ConfigWriter
	ConfigReader
}
