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

// UncommittedDocumentObjectCleaner removes an uploaded object only when no
// Document record was ever committed for it. Business deletion remains a
// logical deletion and must not use this port.
type UncommittedDocumentObjectCleaner interface {
	DeleteUncommitted(ctx context.Context, objectKey string) error
}
