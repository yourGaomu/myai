package minioadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestConfigValidateRequiresConnectionAndBucketFields(t *testing.T) {
	valid := Config{
		Endpoint:  "localhost:9000",
		AccessKey: "access-key",
		SecretKey: "secret-key",
		Bucket:    "knowledge",
	}
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "endpoint", mutate: func(config *Config) { config.Endpoint = " " }, want: "endpoint"},
		{name: "access key", mutate: func(config *Config) { config.AccessKey = " " }, want: "access key"},
		{name: "secret key", mutate: func(config *Config) { config.SecretKey = " " }, want: "secret key"},
		{name: "bucket", mutate: func(config *Config) { config.Bucket = " " }, want: "bucket"},
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config returned error: %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			err := config.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q validation error, got %v", test.want, err)
			}
		})
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		useSSL     bool
		wantHost   string
		wantSecure bool
		wantError  bool
	}{
		{name: "host preserves configured ssl", endpoint: " minio.local:9000 ", useSSL: true, wantHost: "minio.local:9000", wantSecure: true},
		{name: "http url disables ssl", endpoint: "http://minio.local:9000", useSSL: true, wantHost: "minio.local:9000", wantSecure: false},
		{name: "https url enables ssl", endpoint: "https://minio.local:9000/", useSSL: false, wantHost: "minio.local:9000", wantSecure: true},
		{name: "rejects unsupported scheme", endpoint: "ftp://minio.local:9000", wantError: true},
		{name: "rejects path", endpoint: "https://minio.local:9000/storage", wantError: true},
		{name: "rejects query", endpoint: "https://minio.local:9000?region=test", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host, secure, err := normalizeEndpoint(test.endpoint, test.useSSL)
			if test.wantError {
				if err == nil {
					t.Fatal("expected endpoint validation error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if host != test.wantHost || secure != test.wantSecure {
				t.Fatalf("got host=%q secure=%v, want host=%q secure=%v", host, secure, test.wantHost, test.wantSecure)
			}
		})
	}
}

func TestStorePutHashesAndUploadsSeekableContent(t *testing.T) {
	operations := &fakeOperations{}
	store := newWithOperations(operations, Config{Bucket: "knowledge"})
	content := []byte("hello minio")

	reference, err := store.Put(context.Background(), "documents/1/source.txt", bytes.NewReader(content), -1, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	wantHashBytes := sha256.Sum256(content)
	wantHash := hex.EncodeToString(wantHashBytes[:])
	if operations.putCalls != 1 || operations.putBucket != "knowledge" || operations.putObjectKey != "documents/1/source.txt" {
		t.Fatalf("unexpected put call: %#v", operations)
	}
	if !bytes.Equal(operations.putContent, content) || operations.putSize != int64(len(content)) {
		t.Fatalf("unexpected uploaded content=%q size=%d", operations.putContent, operations.putSize)
	}
	if operations.putContentType != "text/plain" || operations.putMetadata[contentSHA256Metadata] != wantHash {
		t.Fatalf("unexpected upload options: contentType=%q metadata=%#v", operations.putContentType, operations.putMetadata)
	}
	if reference.ObjectKey != "documents/1/source.txt" || reference.ContentHash != wantHash || reference.Size != int64(len(content)) || reference.ContentType != "text/plain" {
		t.Fatalf("unexpected object reference: %#v", reference)
	}
}

func TestStorePutRejectsSizeMismatchBeforeUpload(t *testing.T) {
	operations := &fakeOperations{}
	store := newWithOperations(operations, Config{Bucket: "knowledge"})

	_, err := store.Put(context.Background(), "documents/1/source.txt", bytes.NewReader([]byte("content")), 99, "text/plain")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected size mismatch error, got %v", err)
	}
	if operations.putCalls != 0 {
		t.Fatalf("size mismatch must not call MinIO, calls=%d", operations.putCalls)
	}
}

func TestStorePutStagesNonSeekableContentAndRemovesTemporaryFile(t *testing.T) {
	operations := &fakeOperations{}
	store := newWithOperations(operations, Config{Bucket: "knowledge"})
	content := []byte("streamed content")

	_, err := store.Put(context.Background(), "documents/2/source.txt", readerOnly{Reader: bytes.NewBuffer(content)}, int64(len(content)), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if !operations.putUsedFile || !bytes.Equal(operations.putContent, content) {
		t.Fatalf("expected staged file upload, usedFile=%v content=%q", operations.putUsedFile, operations.putContent)
	}
	if operations.putFileName == "" {
		t.Fatal("expected temporary file name")
	}
	if _, err := os.Stat(operations.putFileName); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary upload file was not removed: %v", err)
	}
}

func TestStoreStatReadsContentHashMetadataCaseInsensitively(t *testing.T) {
	metadataKeys := []string{
		"myai-content-sha256",
		"MyAI-Content-SHA256",
		"X-Amz-Meta-Myai-Content-Sha256",
	}
	for _, metadataKey := range metadataKeys {
		t.Run(metadataKey, func(t *testing.T) {
			operations := &fakeOperations{statInfo: storedObjectInfo{
				ObjectKey:   "documents/1/source.txt",
				Size:        12,
				ContentType: "text/plain",
				Metadata:    map[string]string{metadataKey: "content-hash"},
			}}
			store := newWithOperations(operations, Config{Bucket: "knowledge"})

			info, err := store.Stat(context.Background(), "documents/1/source.txt")
			if err != nil {
				t.Fatal(err)
			}
			if info.ContentHash != "content-hash" || info.ObjectKey != "documents/1/source.txt" || info.Size != 12 || info.ContentType != "text/plain" {
				t.Fatalf("unexpected object info: %#v", info)
			}
		})
	}
}

func TestStoreEnsureBucket(t *testing.T) {
	t.Run("existing bucket", func(t *testing.T) {
		operations := &fakeOperations{bucketExists: true}
		store := newWithOperations(operations, Config{Bucket: "knowledge", AutoCreateBucket: true})
		if err := store.EnsureBucket(context.Background()); err != nil {
			t.Fatal(err)
		}
		if operations.makeBucketCalls != 0 {
			t.Fatalf("existing bucket must not be created, calls=%d", operations.makeBucketCalls)
		}
	})

	t.Run("creates missing bucket", func(t *testing.T) {
		operations := &fakeOperations{}
		store := newWithOperations(operations, Config{Bucket: "knowledge", Region: "cn-east-1", AutoCreateBucket: true})
		if err := store.EnsureBucket(context.Background()); err != nil {
			t.Fatal(err)
		}
		if operations.makeBucketCalls != 1 || operations.makeBucketName != "knowledge" || operations.makeBucketRegion != "cn-east-1" {
			t.Fatalf("unexpected make bucket call: %#v", operations)
		}
	})

	t.Run("rejects missing bucket when auto create disabled", func(t *testing.T) {
		operations := &fakeOperations{}
		store := newWithOperations(operations, Config{Bucket: "knowledge"})
		err := store.EnsureBucket(context.Background())
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("expected missing bucket error, got %v", err)
		}
		if operations.makeBucketCalls != 0 {
			t.Fatalf("auto create disabled must not create bucket, calls=%d", operations.makeBucketCalls)
		}
	})
}

func TestStoreOpenWrapsOperationError(t *testing.T) {
	sentinel := errors.New("object unavailable")
	store := newWithOperations(&fakeOperations{openErr: sentinel}, Config{Bucket: "knowledge"})

	_, err := store.Open(context.Background(), "documents/1/source.txt")
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "documents/1/source.txt") {
		t.Fatalf("expected wrapped object error, got %v", err)
	}
}

func TestStoreDeletesOnlyExplicitlyUncommittedObject(t *testing.T) {
	operations := &fakeOperations{}
	store := newWithOperations(operations, Config{Bucket: "knowledge"})

	if err := store.DeleteUncommitted(context.Background(), "documents/1/source.txt"); err != nil {
		t.Fatal(err)
	}
	if operations.removeCalls != 1 || operations.removeKey != "documents/1/source.txt" {
		t.Fatalf("unexpected remove call: %#v", operations)
	}
}

type readerOnly struct {
	io.Reader
}

type fakeOperations struct {
	putCalls       int
	putBucket      string
	putObjectKey   string
	putContent     []byte
	putSize        int64
	putContentType string
	putMetadata    map[string]string
	putUsedFile    bool
	putFileName    string
	putErr         error

	openContent io.ReadCloser
	openErr     error
	statInfo    storedObjectInfo
	statErr     error
	removeCalls int
	removeKey   string
	removeErr   error

	bucketExists     bool
	bucketExistsErr  error
	makeBucketCalls  int
	makeBucketName   string
	makeBucketRegion string
	makeBucketErr    error
}

func (operations *fakeOperations) PutObject(_ context.Context, bucket string, objectKey string, content io.Reader, size int64, contentType string, metadata map[string]string) (uploadInfo, error) {
	operations.putCalls++
	operations.putBucket = bucket
	operations.putObjectKey = objectKey
	operations.putSize = size
	operations.putContentType = contentType
	operations.putMetadata = cloneMetadata(metadata)
	if file, ok := content.(*os.File); ok {
		operations.putUsedFile = true
		operations.putFileName = file.Name()
	}
	uploaded, err := io.ReadAll(content)
	if err != nil {
		return uploadInfo{}, err
	}
	operations.putContent = uploaded
	if operations.putErr != nil {
		return uploadInfo{}, operations.putErr
	}
	return uploadInfo{Size: int64(len(uploaded))}, nil
}

func (operations *fakeOperations) OpenObject(context.Context, string, string) (io.ReadCloser, error) {
	if operations.openErr != nil {
		return nil, operations.openErr
	}
	if operations.openContent != nil {
		return operations.openContent, nil
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (operations *fakeOperations) StatObject(context.Context, string, string) (storedObjectInfo, error) {
	return operations.statInfo, operations.statErr
}

func (operations *fakeOperations) RemoveObject(_ context.Context, _ string, objectKey string) error {
	operations.removeCalls++
	operations.removeKey = objectKey
	return operations.removeErr
}

func (operations *fakeOperations) BucketExists(context.Context, string) (bool, error) {
	return operations.bucketExists, operations.bucketExistsErr
}

func (operations *fakeOperations) MakeBucket(_ context.Context, bucket string, region string) error {
	operations.makeBucketCalls++
	operations.makeBucketName = bucket
	operations.makeBucketRegion = region
	return operations.makeBucketErr
}

func cloneMetadata(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
