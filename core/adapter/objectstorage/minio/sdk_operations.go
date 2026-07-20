package minioadapter

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type sdkOperations struct {
	client *miniosdk.Client
}

func newSDKOperations(config Config) (*sdkOperations, error) {
	endpoint, secure, err := normalizeEndpoint(config.Endpoint, config.UseSSL)
	if err != nil {
		return nil, err
	}
	client, err := miniosdk.New(endpoint, &miniosdk.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, config.SessionToken),
		Secure: secure,
		Region: strings.TrimSpace(config.Region),
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &sdkOperations{client: client}, nil
}

func (operations *sdkOperations) PutObject(ctx context.Context, bucket string, objectKey string, content io.Reader, size int64, contentType string, metadata map[string]string) (uploadInfo, error) {
	info, err := operations.client.PutObject(ctx, bucket, objectKey, content, size, miniosdk.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: metadata,
	})
	if err != nil {
		return uploadInfo{}, err
	}
	return uploadInfo{Size: info.Size}, nil
}

func (operations *sdkOperations) OpenObject(ctx context.Context, bucket string, objectKey string) (io.ReadCloser, error) {
	if _, err := operations.client.StatObject(ctx, bucket, objectKey, miniosdk.StatObjectOptions{}); err != nil {
		return nil, err
	}
	object, err := operations.client.GetObject(ctx, bucket, objectKey, miniosdk.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return object, nil
}

func (operations *sdkOperations) StatObject(ctx context.Context, bucket string, objectKey string) (storedObjectInfo, error) {
	info, err := operations.client.StatObject(ctx, bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		return storedObjectInfo{}, err
	}
	metadata := make(map[string]string, len(info.UserMetadata)+len(info.Metadata))
	for key, value := range info.UserMetadata {
		metadata[key] = value
	}
	for key, values := range info.Metadata {
		if len(values) > 0 {
			metadata[key] = values[0]
		}
	}
	return storedObjectInfo{
		ObjectKey:   info.Key,
		Size:        info.Size,
		ContentType: info.ContentType,
		Metadata:    metadata,
	}, nil
}

func (operations *sdkOperations) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return operations.client.BucketExists(ctx, bucket)
}

func (operations *sdkOperations) MakeBucket(ctx context.Context, bucket string, region string) error {
	return operations.client.MakeBucket(ctx, bucket, miniosdk.MakeBucketOptions{Region: region})
}

func normalizeEndpoint(endpoint string, useSSL bool) (string, bool, error) {
	endpoint = strings.TrimSpace(endpoint)
	if !strings.Contains(endpoint, "://") {
		return endpoint, useSSL, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("parse minio endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false, fmt.Errorf("minio endpoint scheme must be http or https")
	}
	if parsed.Host == "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("minio endpoint must not contain a path, query, or fragment")
	}
	return parsed.Host, parsed.Scheme == "https", nil
}
