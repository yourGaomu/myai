package repository

import "context"

type AssetRepository interface {
	SaveAsset(ctx context.Context, asset AssetRecord) error
	ListAssets(ctx context.Context, sessionID string, limit int) ([]AssetRecord, error)
}

// SessionAttachmentReader checks one session/asset pair even when it no longer
// appears on the latest page of the asset list.
type SessionAttachmentReader interface {
	FindSessionAttachment(ctx context.Context, sessionID, assetID string) (AssetRecord, error)
}
