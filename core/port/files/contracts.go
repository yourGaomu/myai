package files

import (
	"context"
	"errors"
)

var ErrSessionAssetUnavailable = errors.New("attachment is unavailable in this session")

var (
	ErrUploadServiceUnavailable = errors.New("file upload service is unavailable")
	ErrUploadMissingTool        = errors.New("file upload service is missing a required tool")
	ErrUploadUpstreamFailure    = errors.New("file upload failed at the upstream service")
)

// UploadPreflighter is an optional Service capability for checking whether new
// uploads can start. It must not create a backend upload or consume file data.
type UploadPreflighter interface {
	PreflightUpload(ctx context.Context) error
}

// SessionAssetLookup admits assets uploaded or attached to the given session.
// The calling code uses the returned metadata to check the downloaded object.
type SessionAssetLookup func(ctx context.Context, sessionID, assetID string) (Asset, error)

func IsSessionAssetID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

// UploadCanceller resolves an upload by its internal idempotency key on the
// original backend. A nonempty asset means completion won the race. An error
// means cancellation is unconfirmed and must be retried with the same key.
type UploadCanceller interface {
	CancelUpload(ctx context.Context, idempotencyKey string) (Asset, error)
}

type Service interface {
	Upload(context.Context, UploadRequest) (Asset, error)
	Share(context.Context, ShareRequest) (Asset, error)
	Info(context.Context, string) (Asset, error)
	Download(context.Context, DownloadRequest) (Download, error)
}


// SessionAssetRegistrar binds locally stored image bytes to a live session so
// they remain addressable when no remote files service is configured.
type SessionAssetRegistrar interface {
	RegisterLocalSessionAsset(ctx context.Context, sessionID string, asset Asset) error
}
