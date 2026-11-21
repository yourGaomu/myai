package agent

import (
	"context"
	"errors"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/remote/protocol"
)

const assetImagePreviewTimeout = 18 * time.Second
const maxConcurrentAssetPreviews = 2

type sessionImagePreviewer interface {
	SessionImagePreview(ctx context.Context, sessionID, assetID string) ([]byte, int, int, error)
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
		data, width, height, err := previewer.SessionImagePreview(previewCtx, message.SessionID, payload.AssetID)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if a.writeHandlerError(conn, message, err) != nil {
				_ = conn.Close()
			}
			return
		}
		result := protocol.AssetImagePreviewResultPayload{
			AssetID: payload.AssetID, ContentType: "image/jpeg", Data: data, Width: width, Height: height,
		}
		if a.writeRemoteMessage(conn, protocol.TypeAssetImagePreviewResult, message.RequestID, message.SessionID, result) != nil {
			_ = conn.Close()
		}
	}()
	return nil
}
