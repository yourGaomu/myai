package model

import (
	"myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
)

type ModelInfo struct {
	ID                        string
	Name                      string
	Provider                  string
	Protocol                  domainmodel.Protocol
	AuthType                  domainmodel.AuthType
	BaseURL                   string
	HasAPIKey                 bool
	ModelName                 string
	Enabled                   bool
	IsDefault                 bool
	DefaultGenerationSettings generation.Settings
}
