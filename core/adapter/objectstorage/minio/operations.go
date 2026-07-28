package minioadapter

import (
	"context"
	"io"
)

type operations interface {
	PutObject(ctx context.Context, bucket string, objectKey string, content io.Reader, size int64, contentType string, metadata map[string]string) (uploadInfo, error)
	OpenObject(ctx context.Context, bucket string, objectKey string) (io.ReadCloser, error)
	StatObject(ctx context.Context, bucket string, objectKey string) (storedObjectInfo, error)
	RemoveObject(ctx context.Context, bucket string, objectKey string) error
	BucketExists(ctx context.Context, bucket string) (bool, error)
	MakeBucket(ctx context.Context, bucket string, region string) error
}
