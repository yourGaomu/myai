package agent

import (
	"context"
	"errors"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/imageasset"
	"myai/core/port/files"
	"myai/core/remote/protocol"
	"myai/core/service"
)

const assetImagePreviewTimeout = 18 * time.Second
const maxConcurrentAssetPreviews = 2

type sessionImagePreviewer interface {
	SessionImagePreview(ctx context.Context, sessionID, assetID string) ([]byte, string, int, int, error)
}

func (a *Agent) startAssetImagePreview(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.AssetImagePreviewRequest(message)
	if err != nil {
		return err
	}
	if _, err := a.uploadOwner(message); err != nil {
		return errors.New("image preview requires an authenticated paired client")
	}
	previewer, ok := a.chatService.(sessionImagePreviewer)
	if !ok {
		return errors.New("image preview is not available on this Agent")
	}
	if a.activeImagePreviews.Add(1) > maxConcurrentAssetPreviews {
		a.activeImagePreviews.Add(-1)
		return errors.New("too many image previews in progress; retry shortly")
	}
	a.tasks.Add(1)
	go func() {
		defer a.tasks.Done()
		defer a.activeImagePreviews.Add(-1)
		previewCtx, cancel := context.WithTimeout(ctx, assetImagePreviewTimeout)
		defer cancel()
		data, contentType, width, height, err := previewer.SessionImagePreview(previewCtx, message.SessionID, payload.AssetID)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			failure := assetPreviewErrorPayload(err)
			if a.writeRemoteMessage(conn, protocol.TypeError, message.RequestID, message.SessionID, failure) != nil {
				_ = conn.Close()
			}
			return
		}
		result := protocol.AssetImagePreviewResultPayload{
			AssetID: payload.AssetID, ContentType: contentType, Data: data, Width: width, Height: height,
		}
		if a.writeRemoteMessage(conn, protocol.TypeAssetImagePreviewResult, message.RequestID, message.SessionID, result) != nil {
			_ = conn.Close()
		}
	}()
	return nil
}

func assetPreviewErrorPayload(err error) protocol.AssetImagePreviewErrorPayload {
	payload := protocol.ErrorPayload{Message: "image preview is unavailable"}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		payload = protocol.ErrorPayload{Code: protocol.AssetPreviewErrorTimeout, Message: "image preview timed out"}
	case errors.Is(err, files.ErrSessionAssetUnavailable):
		payload = protocol.ErrorPayload{Code: protocol.AssetPreviewErrorAttachmentUnavailable, Message: "the image is unavailable in this session"}
	case errors.Is(err, service.ErrImagePreviewDownload):
		payload = protocol.ErrorPayload{Code: protocol.AssetPreviewErrorDownload, Message: "the Agent could not read the image from object storage"}
	case errors.Is(err, imageasset.ErrUnsupported):
		payload = protocol.ErrorPayload{Code: protocol.AssetPreviewErrorInvalidImage, Message: "the stored image cannot be decoded"}
	case errors.Is(err, imageasset.ErrTooLarge):
		payload = protocol.ErrorPayload{Code: protocol.AssetPreviewErrorTooLarge, Message: "the image exceeds preview limits"}
	}
	return protocol.AssetImagePreviewErrorPayload{ErrorPayload: payload, RequestType: protocol.TypeAssetImagePreview}
}
