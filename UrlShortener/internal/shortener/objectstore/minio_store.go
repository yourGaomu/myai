package objectstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOOptions struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	UseSSL          bool
	EnsureBucket    bool
}

type MinIOStore struct {
	client *minio.Client
	bucket string
}

func NewMinIOStore(ctx context.Context, options MinIOOptions) (*MinIOStore, error) {
	endpoint, secure, err := normalizeEndpoint(options.Endpoint, options.UseSSL)
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		return nil, errors.New("minio endpoint is required")
	}
	if options.AccessKeyID == "" {
		return nil, errors.New("minio access key is required")
	}
	if options.SecretAccessKey == "" {
		return nil, errors.New("minio secret key is required")
	}
	if options.Bucket == "" {
		return nil, errors.New("minio bucket is required")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(options.AccessKeyID, options.SecretAccessKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, err
	}

	if options.EnsureBucket {
		exists, err := client.BucketExists(ctx, options.Bucket)
		if err != nil {
			return nil, err
		}
		if !exists {
			if err := client.MakeBucket(ctx, options.Bucket, minio.MakeBucketOptions{}); err != nil {
				return nil, err
			}
		}
	}

	return &MinIOStore{
		client: client,
		bucket: options.Bucket,
	}, nil
}

func (s *MinIOStore) Upload(ctx context.Context, request UploadRequest) (ObjectInfo, error) {
	if s.client == nil {
		return ObjectInfo{}, errors.New("minio client is nil")
	}
	if request.Reader == nil {
		return ObjectInfo{}, errors.New("upload reader is nil")
	}
	if request.Size <= 0 {
		return ObjectInfo{}, errors.New("upload file is empty")
	}

	fileName := sanitizeFileName(request.FileName)
	objectKey, err := newObjectKey(fileName)
	if err != nil {
		return ObjectInfo{}, err
	}
	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	info, err := s.client.PutObject(ctx, s.bucket, objectKey, request.Reader, request.Size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return ObjectInfo{}, err
	}

	return ObjectInfo{
		Bucket:      info.Bucket,
		Key:         info.Key,
		FileName:    fileName,
		ContentType: contentType,
		Size:        request.Size,
	}, nil
}

func (s *MinIOStore) PresignedGetURL(ctx context.Context, bucket string, key string, expires time.Duration) (string, error) {
	if s.client == nil {
		return "", errors.New("minio client is nil")
	}
	if bucket == "" {
		bucket = s.bucket
	}
	if key == "" {
		return "", errors.New("minio object key is required")
	}
	if expires <= 0 {
		expires = time.Hour
	}

	presignedURL, err := s.client.PresignedGetObject(ctx, bucket, key, expires, nil)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func normalizeEndpoint(endpoint string, useSSL bool) (string, bool, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", useSSL, nil
	}
	if !strings.Contains(endpoint, "://") {
		return endpoint, useSSL, nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", useSSL, err
	}
	if parsed.Host == "" {
		return "", useSSL, fmt.Errorf("invalid minio endpoint: %s", endpoint)
	}
	return parsed.Host, parsed.Scheme == "https", nil
}

func sanitizeFileName(name string) string {
	name = path.Base(strings.TrimSpace(name))
	if name == "." || name == "/" || name == "" {
		return "file"
	}
	return strings.ReplaceAll(name, "\\", "_")
}

func newObjectKey(fileName string) (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	return fmt.Sprintf(
		"uploads/%04d/%02d/%02d/%s-%s",
		now.Year(),
		now.Month(),
		now.Day(),
		hex.EncodeToString(randomBytes),
		fileName,
	), nil
}
