package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"myai/core/imageasset"
	files "myai/core/port/files"
)

// LookupSessionAsset also fences attachment use during a clear/delete. Generation
// already holds its session operation lock, so this read must not take it again.
func (s *ChatService) LookupSessionAsset(ctx context.Context, sessionID, assetID string) (files.Asset, error) {
	if s == nil || !files.IsSessionAssetID(assetID) || strings.TrimSpace(sessionID) == "" {
		return files.Asset{}, files.ErrSessionAssetUnavailable
	}
	before, err := s.assetReadEpoch(sessionID)
	if err != nil {
		return files.Asset{}, err
	}
	if localAsset, ok := s.localSessionAsset(sessionID, assetID); ok {
		session, err := s.dependencies.SessionLoader.Load(ctx, sessionID)
		if err != nil || session == nil || session.ID != sessionID || session.WorkspaceRoot != "" && filepath.Clean(session.WorkspaceRoot) != filepath.Clean(localAsset.WorkspaceRoot()) {
			return files.Asset{}, files.ErrSessionAssetUnavailable
		}
		if current, err := s.assetReadEpoch(sessionID); err != nil || current != before {
			return files.Asset{}, files.ErrSessionAssetUnavailable
		}
		return localAsset, nil
	}
	if s.dependencies.LookupSessionAsset == nil {
		return files.Asset{}, files.ErrSessionAssetUnavailable
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

// RegisterLocalSessionAsset makes a checksummed workspace image addressable by
// its creating session when no external file service is configured.
func (s *ChatService) RegisterLocalSessionAsset(ctx context.Context, sessionID string, asset files.Asset) error {
	sessionID = strings.TrimSpace(sessionID)
	if s == nil || sessionID == "" || !strings.HasPrefix(asset.AssetID, "ast_local_") || !files.IsSessionAssetID(asset.AssetID) || asset.Size <= 0 || asset.Size > imageasset.MaxSourceBytes {
		return files.ErrSessionAssetUnavailable
	}
	if s.dependencies.SessionLoader == nil {
		return files.ErrSessionAssetUnavailable
	}
	epoch, err := s.assetReadEpoch(sessionID)
	if err != nil {
		return err
	}
	current, err := s.dependencies.SessionLoader.Load(ctx, sessionID)
	if err != nil || current == nil || current.ID != sessionID {
		return files.ErrSessionAssetUnavailable
	}
	if current.WorkspaceRoot != "" {
		if asset.WorkspaceRoot() != "" && filepath.Clean(asset.WorkspaceRoot()) != filepath.Clean(current.WorkspaceRoot) {
			return files.ErrSessionAssetUnavailable
		}
		asset = asset.WithLocalImage(asset.LocalPath(), current.WorkspaceRoot)
	}
	if strings.TrimSpace(asset.WorkspaceRoot()) == "" || strings.TrimSpace(asset.LocalPath()) == "" || strings.TrimSpace(asset.Checksum) == "" {
		return files.ErrSessionAssetUnavailable
	}
	data, err := readSessionLocalAsset(ctx, asset)
	if err != nil {
		return err
	}
	info, err := imageasset.Inspect(data)
	if err != nil || info.MIMEType != asset.ContentType {
		return files.ErrSessionAssetUnavailable
	}
	if currentEpoch, err := s.assetReadEpoch(sessionID); err != nil || currentEpoch != epoch {
		return files.ErrSessionAssetUnavailable
	}
	s.localAssetMu.Lock()
	if s.localAssets == nil {
		s.localAssets = make(map[string]map[string]files.Asset)
	}
	if s.localAssets[sessionID] == nil {
		s.localAssets[sessionID] = make(map[string]files.Asset)
	}
	s.localAssets[sessionID][asset.AssetID] = asset
	s.localAssetMu.Unlock()
	return nil
}

func (s *ChatService) localSessionAsset(sessionID, assetID string) (files.Asset, bool) {
	s.localAssetMu.RLock()
	defer s.localAssetMu.RUnlock()
	asset, ok := s.localAssets[sessionID][assetID]
	return asset, ok
}

func (s *ChatService) clearLocalSessionAssets(sessionID string) {
	s.localAssetMu.Lock()
	delete(s.localAssets, sessionID)
	s.localAssetMu.Unlock()
}

func (s *ChatService) assetReadEpoch(sessionID string) (uint64, error) {
	s.generationMu.Lock()
	defer s.generationMu.Unlock()
	if s.sessionPersistenceBlockedLocked(sessionID) != nil {
		return 0, files.ErrSessionAssetUnavailable
	}
	return s.fileUploadEpochs[sessionID], nil
}

// ErrImagePreviewDownload means the authorized image could not be recovered from object storage.
var ErrImagePreviewDownload = errors.New("could not download the attached image")

// SessionImagePreview verifies an authorized local copy before requesting the stored object.
func (s *ChatService) SessionImagePreview(ctx context.Context, sessionID, assetID string) ([]byte, string, int, int, error) {
	if s == nil {
		return nil, "", 0, 0, errors.New("image preview is not available on this Agent")
	}
	epoch, err := s.assetReadEpoch(sessionID)
	if err != nil {
		return nil, "", 0, 0, err
	}
	asset, err := s.LookupSessionAsset(ctx, sessionID, assetID)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if asset.Size <= 0 || asset.Size > imageasset.MaxSourceBytes {
		return nil, "", 0, 0, imageasset.ErrTooLarge
	}

	source, err := readSessionLocalAsset(ctx, asset)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", 0, 0, ctx.Err()
		}
		if strings.HasPrefix(assetID, "ast_local_") || s.dependencies.AssetFiles == nil {
			return nil, "", 0, 0, files.ErrSessionAssetUnavailable
		}
		source, err = s.downloadSessionPreviewSource(ctx, asset)
		if err != nil {
			return nil, "", 0, 0, err
		}
	}
	data, contentType, width, height, err := imageasset.PreviewBytes(ctx, source)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if current, err := s.assetReadEpoch(sessionID); err != nil || current != epoch {
		return nil, "", 0, 0, files.ErrSessionAssetUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, "", 0, 0, err
	}
	return data, contentType, width, height, nil
}

func (s *ChatService) downloadSessionPreviewSource(ctx context.Context, asset files.Asset) ([]byte, error) {
	object, err := s.dependencies.AssetFiles.Download(ctx, files.DownloadRequest{AssetID: asset.AssetID, MaxBytes: imageasset.MaxSourceBytes})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrImagePreviewDownload, err)
	}
	if object.Truncated || len(object.Data) > imageasset.MaxSourceBytes {
		return nil, imageasset.ErrTooLarge
	}
	if object.Asset.AssetID != asset.AssetID || int64(len(object.Data)) != asset.Size ||
		(asset.BusinessID != "" && asset.BusinessID != object.Asset.BusinessID) ||
		(asset.Checksum != "" && !strings.EqualFold(asset.Checksum, object.Asset.Checksum)) {
		return nil, fmt.Errorf("%w: object does not match the session upload record", ErrImagePreviewDownload)
	}
	return object.Data, nil
}

func readSessionLocalAsset(ctx context.Context, asset files.Asset) ([]byte, error) {
	if strings.TrimSpace(asset.WorkspaceRoot()) == "" || strings.TrimSpace(asset.LocalPath()) == "" || strings.TrimSpace(asset.Checksum) == "" {
		return nil, files.ErrSessionAssetUnavailable
	}
	slash := strings.ReplaceAll(asset.LocalPath(), "\\", "/")
	for _, component := range strings.Split(slash, "/") {
		if component == ".." || component == "" {
			return nil, files.ErrSessionAssetUnavailable
		}
	}
	relative := filepath.Clean(filepath.FromSlash(slash))
	if filepath.IsAbs(relative) || !filepath.IsLocal(relative) || relative == "." {
		return nil, files.ErrSessionAssetUnavailable
	}
	workspace := filepath.Clean(asset.WorkspaceRoot())
	rootInfo, err := os.Lstat(workspace)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil, files.ErrSessionAssetUnavailable
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, files.ErrSessionAssetUnavailable
	}
	defer func() { _ = root.Close() }()
	openedRoot, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, openedRoot) {
		return nil, files.ErrSessionAssetUnavailable
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for _, component := range parts[:len(parts)-1] {
		before, err := root.Lstat(component)
		if err != nil || !before.IsDir() || before.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return nil, files.ErrSessionAssetUnavailable
		}
		next, err := root.OpenRoot(component)
		if err != nil {
			return nil, files.ErrSessionAssetUnavailable
		}
		current, err := next.Stat(".")
		if err != nil || !os.SameFile(before, current) {
			_ = next.Close()
			return nil, files.ErrSessionAssetUnavailable
		}
		_ = root.Close()
		root = next
	}
	name := parts[len(parts)-1]
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() != asset.Size || before.Size() > imageasset.MaxSourceBytes {
		return nil, files.ErrSessionAssetUnavailable
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, files.ErrSessionAssetUnavailable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || opened.Size() != asset.Size {
		return nil, files.ErrSessionAssetUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, imageasset.MaxSourceBytes+1))
	if err != nil || int64(len(data)) != asset.Size || len(data) > imageasset.MaxSourceBytes {
		return nil, files.ErrSessionAssetUnavailable
	}
	checksum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(checksum[:]), asset.Checksum) {
		return nil, files.ErrSessionAssetUnavailable
	}
	return data, nil
}
