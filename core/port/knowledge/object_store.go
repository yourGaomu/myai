package knowledge

import (
	"context"
	"io"

	domainknowledge "myai/core/domain/knowledge"
)

type DocumentObjectStore interface {
	Put(ctx context.Context, objectKey string, content io.Reader, size int64, contentType string) (domainknowledge.ObjectRef, error)
	Open(ctx context.Context, objectKey string) (io.ReadCloser, error)
	Stat(ctx context.Context, objectKey string) (domainknowledge.ObjectInfo, error)
}
