package service

import (
	"context"
	"errors"
	"strings"

	"myai/core/imageasset"
	files "myai/core/port/files"
)

// LookupSessionAsset also fences attachment use during a clear/delete. Generation
// already holds its session operation lock, so this read must not take it again.
func (s *ChatService) LookupSessionAsset(ctx context.Context, sessionID, assetID string) (files.Asset, error) {
	if s == nil || s.dependencies.LookupSessionAsset == nil || !files.IsSessionAssetID(assetID) || strings.TrimSpace(sessionID) == "" {
		return files.Asset{}, files.ErrSessionAssetUnavailable
	}
	before, err := s.assetReadEpoch(sessionID)
	if err != nil {
		return files.Asset{}, err
	}
	asset, err := s.dependencies.LookupSessionAsset(ctx, sessionID, assetID)
	if err != nil {
		return files.Asset{}, err
	}
	after, err := s.assetReadEpoch(sessionID)
	if err != nil || before != after {
		return files.Asset{}, files.ErrSessionAssetUnavailable
	}
	if asset.AssetID != assetID || asset.Size <= 0 {
		return files.Asset{}, files.ErrSessionAssetUnavailable
	}
	if err := ctx.Err(); err != nil {
		return files.Asset{}, err
	}
	return asset, nil
}

func (s *ChatService) assetReadEpoch(sessionID string) (uint64, error) {
	s.generationMu.Lock()
	defer s.generationMu.Unlock()
	if s.sessionPersistenceBlockedLocked(sessionID) != nil {
		return 0, files.ErrSessionAssetUnavailable
	}
	return s.fileUploadEpochs[sessionID], nil
}

// SessionImagePreview returns only a bounded JPEG. The origin URL and original
// file are kept by the files adapter, and the preview is not persisted.
func (s *ChatService) SessionImagePreview(ctx context.Context, sessionID, assetID string) ([]byte, int, int, error) {
	if s == nil || s.dependencies.AssetFiles == nil {
		return nil, 0, 0, errors.New("image preview is not available on this Agent")
	}
	epoch, err := s.assetReadEpoch(sessionID)
	if err != nil {
		return nil, 0, 0, err
	}
	asset, err := s.LookupSessionAsset(ctx, sessionID, assetID)
	if err != nil {
		return nil, 0, 0, err
	}
	if asset.Size > imageasset.MaxSourceBytes {
		return nil, 0, 0, imageasset.ErrTooLarge
	}
	object, err := s.dependencies.AssetFiles.Download(ctx, files.DownloadRequest{AssetID: assetID, MaxBytes: imageasset.MaxSourceBytes})
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, 0, ctx.Err()
		}
		return nil, 0, 0, errors.New("could not read the attached image")
	}
	if object.Truncated || len(object.Data) > imageasset.MaxSourceBytes {
		return nil, 0, 0, imageasset.ErrTooLarge
	}
	if object.Asset.AssetID != assetID || int64(len(object.Data)) != asset.Size ||
		(asset.BusinessID != "" && asset.BusinessID != object.Asset.BusinessID) ||
		(asset.Checksum != "" && !strings.EqualFold(asset.Checksum, object.Asset.Checksum)) {
		return nil, 0, 0, errors.New("the attached image does not match its upload record")
	}
	decoded, _, err := imageasset.Decode(object.Data)
	if err != nil {
		return nil, 0, 0, err
	}
	data, width, height, err := imageasset.Preview(ctx, decoded)
	if err != nil {
		return nil, 0, 0, err
	}
	if current, err := s.assetReadEpoch(sessionID); err != nil || current != epoch {
		return nil, 0, 0, files.ErrSessionAssetUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, 0, err
	}
	return data, width, height, nil
}
