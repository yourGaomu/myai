package port

import "context"

// UsageChecker 防止禁用或删除仍被会话引用的模型。
type UsageChecker interface {
	IsModelInUse(ctx context.Context, modelID string) (bool, error)
}

// DefaultModelUpdater 让模型配置用例同步更新新建会话使用的默认模型。
type DefaultModelUpdater interface {
	SetDefaultModel(modelID string)
}
