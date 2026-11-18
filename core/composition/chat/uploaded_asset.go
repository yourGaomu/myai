package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	generationadapter "myai/core/adapter/chat/generation"
	toolrecordsmapper "myai/core/adapter/persistence/toolrecords/mapper"
	domaintool "myai/core/domain/tool"
	filesport "myai/core/port/files"
	outboxport "myai/core/port/outbox"
)

func uploadedAssetRecorder(configuration Configuration, queue *generationadapter.SessionQueue) func(context.Context, string, string, filesport.Asset) error {
	return func(ctx context.Context, sessionID, requestID string, asset filesport.Asset) error {
		if configuration.Store == nil {
			return errors.New("uploaded asset persistence requires a session backend")
		}
		current, err := configuration.Store.GetSession(ctx, sessionID)
		if err != nil {
			return outboxport.SafeError("read upload session", err)
		}
		if current.Deleted {
			return outboxport.ErrBlocked
		}
		if strings.TrimSpace(asset.AssetID) == "" || strings.TrimSpace(asset.FileName) == "" || asset.Size < 0 || asset.Size > filesport.MaxFileBytes {
			return errors.New("invalid uploaded asset metadata")
		}
		createdAt := asset.CreatedAt.UTC().Truncate(time.Millisecond)
		if createdAt.IsZero() {
			return errors.New("uploaded asset creation time is missing")
		}
		digest := sha256.Sum256([]byte(sessionID + "\x00" + requestID + "\x00" + asset.AssetID))
		recordID := "upload-" + hex.EncodeToString(digest[:])
		records := (toolrecordsmapper.Mapper{}).AssetRecords([]domaintool.SharedAsset{{
			ID: recordID, SessionID: sessionID, RequestID: requestID, ToolName: "asset_upload",
			AssetID: asset.AssetID, BusinessID: asset.BusinessID, Checksum: asset.Checksum,
			FileName: asset.FileName, ContentType: asset.ContentType, Size: asset.Size,
			ShortURL: asset.ShortURL, ShortCode: asset.Code, ExpiresAt: asset.ExpiresAt,
			MaxVisits: asset.MaxVisits, CreatedAt: createdAt,
		}})
		if queue != nil && queue.Durable() {
			return queue.SubmitRecords(ctx, outboxport.Batch{
				Version: outboxport.Version, ID: recordID, SessionID: sessionID, Assets: records,
			})
		}
		if queue == nil {
			return errors.New("uploaded asset persistence queue is unavailable")
		}
		return queue.SubmitAndWait(ctx, sessionID, func() error {
			current, err := configuration.Store.GetSession(ctx, sessionID)
			if err != nil {
				return outboxport.SafeError("read upload session", err)
			}
			if current.Deleted {
				return outboxport.ErrBlocked
			}
			if err := configuration.Store.SaveAsset(ctx, records[0]); err != nil {
				return outboxport.SafeError("save uploaded asset", err)
			}
			return nil
		})
	}
}
