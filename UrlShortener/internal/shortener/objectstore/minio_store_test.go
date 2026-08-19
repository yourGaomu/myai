package objectstore

import (
	"context"
	"net/url"
	"testing"
	"time"
)

func TestPresignedGetURLUsesPublicEndpoint(t *testing.T) {
	store, err := NewMinIOStore(context.Background(), MinIOOptions{
		Endpoint:        "myai-minio:9000",
		PublicEndpoint:  "https://minio.example.com",
		Region:          "us-east-1",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
		Bucket:          "myai-assets",
		EnsureBucket:    false,
	})
	if err != nil {
		t.Fatalf("new minio store: %v", err)
	}

	value, err := store.PresignedGetURL(context.Background(), "", "uploads/report.txt", time.Hour)
	if err != nil {
		t.Fatalf("presign object: %v", err)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("parse presigned url: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "minio.example.com" {
		t.Fatalf("presigned url should use public endpoint, got %s", value)
	}
	if parsed.Path != "/myai-assets/uploads/report.txt" {
		t.Fatalf("unexpected presigned path: %s", parsed.Path)
	}
}
