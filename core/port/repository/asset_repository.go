package repository

import "context"

type AssetRepository interface {
	SaveAsset(ctx context.Context, asset AssetRecord) error
	ListAssets(ctx context.Context, sessionID string, limit int) ([]AssetRecord, error)
}
