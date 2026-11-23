package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/gorilla/websocket"

	"myai/core/port/files"
	"myai/core/remote/protocol"
	"myai/core/remote/uploads"
)

type Option func(*Agent)

// WithFileService attaches the MCP-backed upload port. It is independent of the
// workspace file browser facade passed to New. Uploads never invoke Share.
func WithFileService(service files.Service) Option {
	return func(a *Agent) { a.uploadService = service }
}

// WithFileUploadAdmission validates the session under the application's lifecycle
// lock and captures its epoch. Upload begin fails closed without this callback.
func WithFileUploadAdmission(admission func(context.Context, string) (uint64, error)) Option {
	return func(a *Agent) { a.fileUploadAdmission = admission }
}

// WithFileAssetRecorder records a completed upload in its originating session.
// The recorder must be idempotent for the same sessionID/requestID/epoch/asset. If it
// fails, a repeated complete request retries recording the saved asset without
// uploading to the backend again. requestID is the first completion request ID;
// epoch is captured at begin, allowing clear/delete to reject old completions.
func WithFileAssetRecorder(recorder func(context.Context, string, string, uint64, files.Asset) error) Option {
	return func(a *Agent) { a.fileAssetRecorder = recorder }
}

func (a *Agent) fileUploads() (*uploads.Receiver, error) {
	a.uploadMu.Lock()
	defer a.uploadMu.Unlock()
	if a.uploadService == nil || a.uploadsClosed {
		return nil, files.ErrUploadServiceUnavailable
	}
	if a.uploadReceiver == nil {
		a.uploadReceiver = uploads.New(a.uploadService, a.fileAssetRecorder, uploads.Config{})
	}
	return a.uploadReceiver, nil
}

func (a *Agent) closeFileUploads() error {
	a.uploadMu.Lock()
	a.uploadsClosed = true
	receiver := a.uploadReceiver
	a.uploadMu.Unlock()
	if receiver != nil {
		return receiver.Close()
	}
	return nil
}

func (a *Agent) uploadOwner(message protocol.Message) (uploads.Owner, error) {
	if message.UserID == "" || message.DeviceID == "" ||
		message.UserID != a.config.UserID || message.DeviceID != a.config.DeviceID ||
		strings.TrimSpace(message.ClientToken) == "" || len(message.ClientToken) > 4096 {
		return uploads.Owner{}, errors.New("upload requires the authenticated paired client identity")
	}
	hash := sha256.Sum256([]byte(message.ClientToken))
	return uploads.Owner{
		UserID: message.UserID, DeviceID: message.DeviceID,
		SessionID: message.SessionID, ClientID: hex.EncodeToString(hash[:]),
	}, nil
}

func (a *Agent) enqueueUploadOperation(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateAssetUploadMessage(message); err != nil {
		return err
	}
	if _, err := a.uploadOwner(message); err != nil {
		return err
	}
	return a.uploadOperations.enqueue(ctx, func() {
		if err := a.handleAssetUpload(ctx, conn, message); err != nil {
			if a.writeHandlerError(conn, message, err) != nil {
				_ = conn.Close()
			}
		}
	})
}

func (a *Agent) handleAssetUpload(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	if err := protocol.ValidateAssetUploadMessage(message); err != nil {
		return err
	}
	owner, err := a.uploadOwner(message)
	if err != nil {
		return err
	}
	receiver, err := a.fileUploads()
	if err != nil {
		return err
	}
	var result uploads.Result
	var operation string
	switch message.Type {
	case protocol.TypeAssetUploadBegin:
		payload, err := protocol.DecodeUploadPayload[protocol.AssetUploadBeginPayload](message)
		if err != nil {
			return err
		}
		if a.fileUploadAdmission == nil {
			return errors.New("file upload session admission is not configured")
		}
		epoch, err := a.fileUploadAdmission(ctx, owner.SessionID)
		if err != nil {
			return err
		}
		operation = "begin"
		result, err = receiver.Begin(ctx, owner, uploads.Metadata{
			FileName: payload.FileName, ContentType: payload.ContentType,
			Size: payload.Size, IdempotencyKey: payload.IdempotencyKey, SessionEpoch: epoch,
		})
		if err != nil {
			return err
		}
	case protocol.TypeAssetUploadChunk:
		payload, err := protocol.DecodeUploadPayload[protocol.AssetUploadChunkPayload](message)
		if err != nil {
			return err
		}
		operation = "chunk"
		result, err = receiver.Chunk(ctx, owner, payload.UploadID, payload.Offset, payload.Data)
		if err != nil {
			return err
		}
	case protocol.TypeAssetUploadStatus, protocol.TypeAssetUploadCancel:
		payload, err := protocol.DecodeUploadPayload[protocol.AssetUploadIDPayload](message)
		if err != nil {
			return err
		}
		if message.Type == protocol.TypeAssetUploadCancel {
			operation = "cancel"
			result, err = receiver.Cancel(ctx, owner, payload.UploadID)
		} else {
			operation = "status"
			result, err = receiver.Status(ctx, owner, payload.UploadID)
		}
		if err != nil {
			return err
		}
	case protocol.TypeAssetUploadComplete:
		payload, err := protocol.DecodeUploadPayload[protocol.AssetUploadIDPayload](message)
		if err != nil {
			return err
		}
		// The receiver bounds backend workers separately. Bound reply waiters
		// too, including repeated complete requests against one slow upload.
		if a.activeUploadReplies.Add(1) > asyncOperationLimit {
			a.activeUploadReplies.Add(-1)
			return errors.New("too many pending upload completion replies; retry later")
		}
		completion, err := receiver.Complete(ctx, owner, payload.UploadID, message.RequestID)
		if err != nil {
			a.activeUploadReplies.Add(-1)
			return err
		}
		a.tasks.Add(1)
		go func() {
			defer a.tasks.Done()
			defer a.activeUploadReplies.Add(-1)
			result, err := completion.Wait(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				_ = a.writeHandlerError(conn, message, err)
				return
			}
			if a.writeUploadResult(conn, message, "complete", result) != nil {
				_ = conn.Close()
			}
		}()
		return nil
	default:
		return errors.New("unsupported upload operation")
	}
	return a.writeUploadResult(conn, message, operation, result)
}

func (a *Agent) writeUploadResult(conn *websocket.Conn, message protocol.Message, operation string, result uploads.Result) error {
	payload := protocol.AssetUploadResultPayload{
		Operation: operation, UploadID: result.UploadID, Offset: result.Offset,
		ChunkSize: uploads.ChunkSize, MaxFileBytes: files.MaxFileBytes,
	}
	if result.Asset != nil {
		asset := result.Asset
		payload.Asset = &protocol.UploadedAsset{
			AssetID: asset.AssetID, BusinessID: asset.BusinessID, FileName: asset.FileName,
			ContentType: asset.ContentType, Size: asset.Size, Checksum: asset.Checksum,
			ShortURL: asset.ShortURL, Code: asset.Code, ExpiresAt: asset.ExpiresAt,
			MaxVisits: asset.MaxVisits,
		}
	}
	return a.writeRemoteMessage(conn, protocol.TypeAssetUploadResult, message.RequestID, message.SessionID, payload)
}
