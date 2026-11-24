package protocol

import (
	"errors"
	"strings"

	"myai/core/imageasset"
	"myai/core/port/files"
)

const (
	TypeAssetImagePreview       MessageType = "asset_image_preview"
	TypeAssetImagePreviewResult MessageType = "asset_image_preview_result"

	AssetImagePreviewMaxBytes            = 2 * 1024 * 1024
	AssetImagePreviewMaxDimension        = 2048
	AssetImagePreviewRequestMaxJSONBytes = 8 * 1024
	AssetImagePreviewMaxJSONBytes        = 3 * 1024 * 1024
)

type AssetImagePreviewPayload struct {
	AssetID string `json:"asset_id"`
}

type AssetImagePreviewResultPayload struct {
	AssetID     string `json:"asset_id"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

func AssetImagePreviewRequest(message Message) (AssetImagePreviewPayload, error) {
	if message.Type != TypeAssetImagePreview || len(message.Payload) > 1024 ||
		message.RequestID == "" || len(message.RequestID) > 256 ||
		message.SessionID == "" || len(message.SessionID) > 512 || message.SessionID != strings.TrimSpace(message.SessionID) {
		return AssetImagePreviewPayload{}, errors.New("invalid image preview request")
	}
	payload, err := DecodePayload[AssetImagePreviewPayload](message)
	if err != nil || !files.IsSessionAssetID(payload.AssetID) {
		return AssetImagePreviewPayload{}, errors.New("invalid image attachment ID")
	}
	return payload, nil
}

func AssetImagePreviewResult(message Message) (AssetImagePreviewResultPayload, error) {
	if message.Type != TypeAssetImagePreviewResult || len(message.Payload) > AssetImagePreviewMaxJSONBytes {
		return AssetImagePreviewResultPayload{}, errors.New("invalid image preview result")
	}
	payload, err := DecodePayload[AssetImagePreviewResultPayload](message)
	if err != nil || !files.IsSessionAssetID(payload.AssetID) ||
		len(payload.Data) == 0 || len(payload.Data) > AssetImagePreviewMaxBytes ||
		payload.Width < 1 || payload.Height < 1 || payload.Width > AssetImagePreviewMaxDimension || payload.Height > AssetImagePreviewMaxDimension {
		return AssetImagePreviewResultPayload{}, errors.New("invalid image preview result")
	}
	info, err := imageasset.Inspect(payload.Data)
	if err != nil || info.MIMEType != payload.ContentType || info.Width != payload.Width || info.Height != payload.Height {
		return AssetImagePreviewResultPayload{}, errors.New("invalid image preview data or dimensions")
	}
	decoded, mimeType, err := imageasset.Decode(payload.Data)
	if err != nil || mimeType != payload.ContentType || decoded.Bounds().Dx() != payload.Width || decoded.Bounds().Dy() != payload.Height {
		return AssetImagePreviewResultPayload{}, errors.New("invalid image preview data or dimensions")
	}
	return payload, nil
}
