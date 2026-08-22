package command

import (
	"myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
)

type AddConfig struct {
	ID                        string
	Name                      string
	Provider                  string
	Protocol                  domainmodel.Protocol
	AuthType                  domainmodel.AuthType
	BaseURL                   string
	APIKey                    string
	ModelName                 string
	IsDefault                 bool
	DefaultGenerationSettings generation.Settings
}

// UpdateConfig 修改已有模型的连接信息和模型默认生成参数。
// APIKey 为空表示保留数据库中的旧密钥，避免编辑普通字段时意外清空认证信息。
type UpdateConfig struct {
	ID                        string
	Name                      string
	Provider                  string
	Protocol                  domainmodel.Protocol
	AuthType                  domainmodel.AuthType
	BaseURL                   string
	APIKey                    string
	ModelName                 string
	DefaultGenerationSettings generation.Settings
}

type SetEnabled struct {
	ID      string
	Enabled bool
}

type SetDefault struct {
	ID string
}

type DeleteConfig struct {
	ID string
}
