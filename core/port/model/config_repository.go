package model

import (
	"context"

	domainmodel "myai/core/domain/model"
)

type ConfigWriter interface {
	SaveConfig(ctx context.Context, config domainmodel.Config) error
}

type ConfigReader interface {
	GetConfig(ctx context.Context, modelID string) (domainmodel.Config, error)
	ListConfigs(ctx context.Context) ([]domainmodel.Config, error)
}

type ConfigDeleter interface {
	DeleteConfig(ctx context.Context, modelID string) error
}

type ConfigRepository interface {
	ConfigWriter
	ConfigReader
	ConfigDeleter
}
