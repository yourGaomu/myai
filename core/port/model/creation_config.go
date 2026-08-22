package model

import domainmodel "myai/core/domain/model"

type CreationConfig struct {
	Provider  string
	Protocol  domainmodel.Protocol
	AuthType  domainmodel.AuthType
	APIKey    string
	BaseURL   string
	ModelName string
}
