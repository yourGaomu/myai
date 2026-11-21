package protocol

import (
	"bytes"
	"errors"
	"image/jpeg"
	"strings"

	"myai/core/port/files"
)

const (
	TypeAssetImagePreview       MessageType = "asset_image_preview"
	TypeAssetImagePreviewResult MessageType = "asset_image_preview_result"

	AssetImagePreviewMaxBytes            = 256 * 1024
	AssetImagePreviewMaxDimension        = 1024
	AssetImagePreviewRequestMaxJSONBytes = 8 * 1024
	AssetImagePreviewMaxJSONBytes        = 512 * 1024
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
	if err != nil || !files.IsSessionAssetID(payload.AssetID) || payload.ContentType != "image/jpeg" ||
		len(payload.Data) < 3 || len(payload.Data) > AssetImagePreviewMaxBytes ||
		payload.Data[0] != 0xff || payload.Data[1] != 0xd8 || payload.Data[2] != 0xff ||
		payload.Width < 1 || payload.Height < 1 || payload.Width > AssetImagePreviewMaxDimension || payload.Height > AssetImagePreviewMaxDimension {
		return AssetImagePreviewResultPayload{}, errors.New("invalid image preview result")
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(payload.Data))
	if err != nil || config.Width != payload.Width || config.Height != payload.Height {
		return AssetImagePreviewResultPayload{}, errors.New("invalid image preview dimensions")
	}
	return payload, nil
}
