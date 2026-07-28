package minioadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

const contentSHA256Metadata = "myai-content-sha256"

type Store struct {
	operations       operations
	bucket           string
	region           string
	autoCreateBucket bool
}

var _ knowledgeport.DocumentObjectStore = (*Store)(nil)
var _ knowledgeport.UncommittedDocumentObjectCleaner = (*Store)(nil)

func New(config Config) (*Store, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	client, err := newSDKOperations(config)
	if err != nil {
		return nil, err
	}
	return newWithOperations(client, config), nil
}

func newWithOperations(client operations, config Config) *Store {
	return &Store{
		operations:       client,
		bucket:           strings.TrimSpace(config.Bucket),
		region:           strings.TrimSpace(config.Region),
		autoCreateBucket: config.AutoCreateBucket,
	}
}

func (store *Store) EnsureBucket(ctx context.Context) error {
	if store == nil || store.operations == nil {
		return fmt.Errorf("minio operations are nil")
	}
	exists, err := store.operations.BucketExists(ctx, store.bucket)
	if err != nil {
		return fmt.Errorf("check minio bucket %q: %w", store.bucket, err)
	}
	if exists {
		return nil
	}
	if !store.autoCreateBucket {
		return fmt.Errorf("minio bucket %q does not exist", store.bucket)
	}
	if err := store.operations.MakeBucket(ctx, store.bucket, store.region); err != nil {
		return fmt.Errorf("create minio bucket %q: %w", store.bucket, err)
	}
	return nil
}

func (store *Store) Put(ctx context.Context, objectKey string, content io.Reader, size int64, contentType string) (domainknowledge.ObjectRef, error) {
	if store == nil || store.operations == nil {
		return domainknowledge.ObjectRef{}, fmt.Errorf("minio operations are nil")
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return domainknowledge.ObjectRef{}, fmt.Errorf("object key is required")
	}
	if content == nil {
		return domainknowledge.ObjectRef{}, fmt.Errorf("object content is required")
	}
	if size < -1 {
		return domainknowledge.ObjectRef{}, fmt.Errorf("object size must be -1 or greater")
	}

	prepared, err := prepareUpload(content, size)
	if err != nil {
		return domainknowledge.ObjectRef{}, err
	}
	defer prepared.close()

	metadata := map[string]string{contentSHA256Metadata: prepared.contentHash}
	info, err := store.operations.PutObject(
		ctx,
		store.bucket,
		objectKey,
		prepared.content,
		prepared.size,
		contentType,
		metadata,
	)
	if err != nil {
		return domainknowledge.ObjectRef{}, fmt.Errorf("put minio object %q: %w", objectKey, err)
	}
	if info.Size != 0 && info.Size != prepared.size {
		return domainknowledge.ObjectRef{}, fmt.Errorf("minio uploaded size %d does not match content size %d", info.Size, prepared.size)
	}
	return domainknowledge.ObjectRef{
		ObjectKey:   objectKey,
		ContentHash: prepared.contentHash,
		Size:        prepared.size,
		ContentType: contentType,
	}, nil
}

func (store *Store) Open(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if store == nil || store.operations == nil {
		return nil, fmt.Errorf("minio operations are nil")
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return nil, fmt.Errorf("object key is required")
	}
	content, err := store.operations.OpenObject(ctx, store.bucket, objectKey)
	if err != nil {
		return nil, fmt.Errorf("open minio object %q: %w", objectKey, err)
	}
	return content, nil
}

func (store *Store) Stat(ctx context.Context, objectKey string) (domainknowledge.ObjectInfo, error) {
	if store == nil || store.operations == nil {
		return domainknowledge.ObjectInfo{}, fmt.Errorf("minio operations are nil")
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return domainknowledge.ObjectInfo{}, fmt.Errorf("object key is required")
	}
	info, err := store.operations.StatObject(ctx, store.bucket, objectKey)
	if err != nil {
		return domainknowledge.ObjectInfo{}, fmt.Errorf("stat minio object %q: %w", objectKey, err)
	}
	return domainknowledge.ObjectInfo{
		ObjectKey:   info.ObjectKey,
		ContentHash: metadataValue(info.Metadata, contentSHA256Metadata),
		Size:        info.Size,
		ContentType: info.ContentType,
	}, nil
}

func (store *Store) DeleteUncommitted(ctx context.Context, objectKey string) error {
	if store == nil || store.operations == nil {
		return fmt.Errorf("minio operations are nil")
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return fmt.Errorf("object key is required")
	}
	if err := store.operations.RemoveObject(ctx, store.bucket, objectKey); err != nil {
		return fmt.Errorf("remove uncommitted minio object %q: %w", objectKey, err)
	}
	return nil
}

type preparedUpload struct {
	content     io.Reader
	size        int64
	contentHash string
	cleanup     func()
}

func (upload preparedUpload) close() {
	if upload.cleanup != nil {
		upload.cleanup()
	}
}

func prepareUpload(content io.Reader, expectedSize int64) (preparedUpload, error) {
	if seeker, ok := content.(io.ReadSeeker); ok {
		return prepareSeekableUpload(seeker, expectedSize)
	}
	return prepareTemporaryUpload(content, expectedSize)
}

func prepareSeekableUpload(content io.ReadSeeker, expectedSize int64) (preparedUpload, error) {
	start, err := content.Seek(0, io.SeekCurrent)
	if err != nil {
		return preparedUpload{}, fmt.Errorf("read upload position: %w", err)
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, content)
	if err != nil {
		return preparedUpload{}, fmt.Errorf("hash upload content: %w", err)
	}
	if _, err := content.Seek(start, io.SeekStart); err != nil {
		return preparedUpload{}, fmt.Errorf("reset upload content: %w", err)
	}
	if err := validatePreparedSize(expectedSize, size); err != nil {
		return preparedUpload{}, err
	}
	return preparedUpload{
		content:     io.LimitReader(content, size),
		size:        size,
		contentHash: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func prepareTemporaryUpload(content io.Reader, expectedSize int64) (preparedUpload, error) {
	file, err := os.CreateTemp("", "myai-knowledge-upload-*")
	if err != nil {
		return preparedUpload{}, fmt.Errorf("create temporary upload file: %w", err)
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hasher), content)
	if err != nil {
		cleanup()
		return preparedUpload{}, fmt.Errorf("stage upload content: %w", err)
	}
	if err := validatePreparedSize(expectedSize, size); err != nil {
		cleanup()
		return preparedUpload{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return preparedUpload{}, fmt.Errorf("reset temporary upload file: %w", err)
	}
	return preparedUpload{
		content:     file,
		size:        size,
		contentHash: hex.EncodeToString(hasher.Sum(nil)),
		cleanup:     cleanup,
	}, nil
}

func validatePreparedSize(expectedSize int64, actualSize int64) error {
	if expectedSize >= 0 && expectedSize != actualSize {
		return fmt.Errorf("object size %d does not match content size %d", expectedSize, actualSize)
	}
	return nil
}

func metadataValue(metadata map[string]string, key string) string {
	for current, value := range metadata {
		trimmed := strings.TrimPrefix(strings.ToLower(current), "x-amz-meta-")
		if trimmed == strings.ToLower(key) {
			return value
		}
	}
	return ""
}
