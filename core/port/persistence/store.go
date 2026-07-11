package persistence

import (
	modelport "myai/core/port/model"
	repository "myai/core/port/repository"
)

type Store interface {
	// Store 聚合项目当前需要的仓库能力，调用方仍应优先依赖更小的专用接口。
	repository.Store
	modelport.ConfigRepository
}
