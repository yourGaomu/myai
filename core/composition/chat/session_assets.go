package chat

import (
	"context"
	"strings"

	generationadapter "myai/core/adapter/chat/generation"
	files "myai/core/port/files"
	repository "myai/core/port/repository"
)

func sessionAssetLookup(store repository.SessionRepository, reader repository.SessionAttachmentReader, queue *generationadapter.SessionQueue) files.SessionAssetLookup {
	return func(ctx context.Context, sessionID, assetID string) (files.Asset, error) {
		if store == nil || strings.TrimSpace(sessionID) == "" || !files.IsSessionAssetID(assetID) {
			return files.Asset{}, files.ErrSessionAssetUnavailable
		}
		current, err := store.GetSession(ctx, sessionID)
		if err != nil || current.ID != sessionID || current.Deleted {
			if ctx.Err() != nil {
				return files.Asset{}, ctx.Err()
			}
			return files.Asset{}, files.ErrSessionAssetUnavailable
		}
		// Check the durable queue first. If the worker already removed the
		// entry, its write completed before the exact repository lookup below.
		record, pending, err := queue.PendingSessionAttachment(ctx, sessionID, assetID)
		if err != nil {
			if ctx.Err() != nil {
				return files.Asset{}, ctx.Err()
			}
			return files.Asset{}, files.ErrSessionAssetUnavailable
		}
		if !pending {
			if reader == nil {
				return files.Asset{}, files.ErrSessionAssetUnavailable
			}
			record, err = reader.FindSessionAttachment(ctx, sessionID, assetID)
			if err != nil {
				if ctx.Err() != nil {
					return files.Asset{}, ctx.Err()
				}
				return files.Asset{}, files.ErrSessionAssetUnavailable
			}
		}
		if record.AssetID != assetID || record.SessionID != sessionID || !record.IsSessionAttachment() || record.Size <= 0 {
			return files.Asset{}, files.ErrSessionAssetUnavailable
		}
		return files.Asset{
			AssetID: record.AssetID, BusinessID: record.BusinessID, FileName: record.FileName,
			ContentType: record.ContentType, Size: record.Size, Checksum: record.Checksum,
			CreatedAt: record.CreatedAt}.WithLocalImage(record.LocalPath, current.WorkspaceRoot), nil
	}
}
