package port

import (
	"context"

	repository "myai/core/port/repository"
)

type Persistence interface {
	SaveMessage(ctx context.Context, record repository.MessageRecord) error
	SaveAsset(ctx context.Context, record repository.AssetRecord) error
}

type AsyncRunner func(task func())
