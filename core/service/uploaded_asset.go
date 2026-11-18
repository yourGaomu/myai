package service

import (
	"context"
	"errors"
	"strings"

	filesport "myai/core/port/files"
)

// AdmitFileUpload captures the current session epoch before any file bytes are
// accepted. Clear/delete advances it, including when their persistence fails.
func (s *ChatService) AdmitFileUpload(ctx context.Context, sessionID string) (uint64, error) {
	if s == nil || s.dependencies.RecordUploadedAsset == nil || s.dependencies.SessionLoader == nil {
		return 0, errors.New("uploaded asset persistence is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, errors.New("upload session identity is required")
	}
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	defer unlock()
	if _, err := s.dependencies.SessionLoader.Load(ctx, sessionID); err != nil {
		return 0, err
	}
	s.generationMu.Lock()
	defer s.generationMu.Unlock()
	if err := s.sessionPersistenceBlockedLocked(sessionID); err != nil {
		return 0, err
	}
	return s.fileUploadEpochs[sessionID], nil
}

// RecordUploadedAsset shares the session operation fence with clear/delete. The
// receiver retries it with the original admission epoch and asset identity.
func (s *ChatService) RecordUploadedAsset(ctx context.Context, sessionID, requestID string, epoch uint64, asset filesport.Asset) error {
	if s == nil || s.dependencies.RecordUploadedAsset == nil {
		return errors.New("uploaded asset persistence is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	requestID = strings.TrimSpace(requestID)
	if sessionID == "" || requestID == "" || strings.TrimSpace(asset.AssetID) == "" {
		return errors.New("upload session, request and asset identities are required")
	}
	unlock, err := s.lockSessionOperation(ctx, sessionID)
	if err != nil {
		return err
	}
	defer unlock()
	s.generationMu.Lock()
	err = s.sessionPersistenceBlockedLocked(sessionID)
	if err == nil && epoch != s.fileUploadEpochs[sessionID] {
		err = errors.New("upload belongs to an earlier session state; begin a new upload")
	}
	s.generationMu.Unlock()
	if err != nil {
		return err
	}
	return s.dependencies.RecordUploadedAsset(ctx, sessionID, requestID, asset)
}
