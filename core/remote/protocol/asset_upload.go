package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	TypeAssetUploadBegin    MessageType = "asset_upload_begin"
	TypeAssetUploadChunk    MessageType = "asset_upload_chunk"
	TypeAssetUploadStatus   MessageType = "asset_upload_status"
	TypeAssetUploadComplete MessageType = "asset_upload_complete"
	TypeAssetUploadCancel   MessageType = "asset_upload_cancel"
	TypeAssetUploadResult   MessageType = "asset_upload_result"

	AssetUploadChunkSize    = 262144
	AssetUploadMaxFileBytes = 52428800
	AssetUploadMaxJSONBytes = 524288
)

type AssetUploadBeginPayload struct {
	FileName       string `json:"file_name"`
	ContentType    string `json:"content_type"`
	Size           int64  `json:"size"`
	IdempotencyKey string `json:"idempotency_key"`
}

type AssetUploadChunkPayload struct {
	UploadID string `json:"upload_id"`
	Offset   int64  `json:"offset"`
	Data     string `json:"data"`
}

type AssetUploadIDPayload struct {
	UploadID string `json:"upload_id"`
}

type AssetUploadStatusPayload = AssetUploadIDPayload
type AssetUploadCompletePayload = AssetUploadIDPayload
type AssetUploadCancelPayload = AssetUploadIDPayload

// UploadedAsset describes an uploaded file. Uploading alone does not create a
// public share; sharing metadata is optional throughout the transport contract.
type UploadedAsset struct {
	AssetID     string     `json:"asset_id"`
	BusinessID  string     `json:"business_id,omitempty"`
	FileName    string     `json:"file_name"`
	ContentType string     `json:"content_type,omitempty"`
	Size        int64      `json:"size"`
	Checksum    string     `json:"checksum,omitempty"`
	ShortURL    string     `json:"short_url,omitempty"`
	Code        string     `json:"code,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	MaxVisits   int64      `json:"max_visits,omitempty"`
}

type AssetUploadResultPayload struct {
	Operation    string         `json:"operation"`
	UploadID     string         `json:"upload_id"`
	Offset       int64          `json:"offset"`
	ChunkSize    int            `json:"chunk_size"`
	MaxFileBytes int64          `json:"max_file_bytes"`
	Asset        *UploadedAsset `json:"asset,omitempty"`
}

func IsAssetUploadRequest(messageType MessageType) bool {
	switch messageType {
	case TypeAssetUploadBegin, TypeAssetUploadChunk, TypeAssetUploadStatus, TypeAssetUploadComplete, TypeAssetUploadCancel:
		return true
	default:
		return false
	}
}

// DecodeUploadPayload rejects unexpected metadata on chunk/status/complete
// messages instead of silently accepting a changed file name or file size.
func DecodeUploadPayload[T any](message Message) (T, error) {
	var payload T
	decoder := json.NewDecoder(bytes.NewReader(message.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, err
	}
	if bytes.Equal(bytes.TrimSpace(message.Payload), []byte("null")) {
		return payload, errors.New("upload payload must be an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return payload, errors.New("upload payload must contain exactly one object")
	}
	return payload, nil
}

func ValidateAssetUploadMessage(message Message) error {
	if !IsAssetUploadRequest(message.Type) {
		return errors.New("not an asset upload request")
	}
	if len(message.Payload) > AssetUploadMaxJSONBytes {
		return errors.New("upload JSON exceeds 524288 bytes")
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(encoded) > AssetUploadMaxJSONBytes {
		return errors.New("upload JSON exceeds 524288 bytes")
	}
	if message.RequestID == "" || len(message.RequestID) > 256 || message.SessionID == "" || len(message.SessionID) > 512 {
		return errors.New("upload request_id and session_id are required and must be bounded")
	}
	switch message.Type {
	case TypeAssetUploadBegin:
		_, err = DecodeUploadPayload[AssetUploadBeginPayload](message)
	case TypeAssetUploadChunk:
		_, err = DecodeUploadPayload[AssetUploadChunkPayload](message)
	default:
		_, err = DecodeUploadPayload[AssetUploadIDPayload](message)
	}
	if err != nil {
		return fmt.Errorf("invalid upload payload: %w", err)
	}
	return nil
}
