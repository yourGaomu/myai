package objectstore

import (
	"context"
	"io"
	"time"
)

type UploadRequest struct {
	Reader      io.Reader
	FileName    string
	ContentType string
	Size        int64
}

type ObjectInfo struct {
	Bucket      string
	Key         string
	FileName    string
	ContentType string
	Size        int64
}

type Store interface {
	Upload(ctx context.Context, request UploadRequest) (ObjectInfo, error)
	PresignedGetURL(ctx context.Context, bucket string, key string, expires time.Duration) (string, error)
}
