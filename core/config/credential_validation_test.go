package config

import (
	"strings"
	"testing"
)

func TestValidateCredentialsRejectsExamplesWithoutValues(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Properties)
	}{
		{"myai.api_key", func(p *Properties) { p.Model.APIKey = "replace-with-model-api-key" }},
		{"rag.minio.access_key", func(p *Properties) { p.RAG.MinIO.AccessKey = "minioadmin" }},
		{"rag.minio.secret_key", func(p *Properties) { p.RAG.MinIO.SecretKey = "myaiadmin123456" }},
		{"sandbox.opensandbox.api_key", func(p *Properties) { p.Sandbox.OpenSandbox.APIKey = "replace-with-opensandbox-api-key" }},
		{"redis.password", func(p *Properties) { p.Redis.Password = "changeme" }},
		{"rag.embedding.models[0].api_key", func(p *Properties) {
			p.RAG.Embedding.Models = []EmbeddingModelProperties{{APIKey: "replace-with-embedding-key"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var properties Properties
			properties.Redis.Address = "redis.example.test:6379"
			properties.RAG.MinIO.Endpoint = "storage.example.test:9000"
			properties.RAG.MinIO.AccessKey, properties.RAG.MinIO.SecretKey = "test-access", "test-secret"
			properties.Sandbox.Provider = "opensandbox"
			properties.Sandbox.OpenSandbox.APIKey = "test-sandbox"
			test.set(&properties)
			err := properties.ValidateCredentials()
			if err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("expected setting-specific validation error, got %v", err)
			}
			if strings.Contains(err.Error(), "replace-with-") || strings.Contains(err.Error(), "myaiadmin123456") {
				t.Fatal("credential value leaked in validation error")
			}
		})
	}
}

func TestValidateCredentialsAllowsDisabledServicesAndLocalModels(t *testing.T) {
	if err := (Properties{}).ValidateCredentials(); err != nil {
		t.Fatal(err)
	}
	disabled := false
	properties := Properties{
		Redis:   RedisProperties{Password: "replace-with-unused-key"},
		Sandbox: SandboxProperties{Provider: "local", OpenSandbox: OpenSandboxProperties{APIKey: "replace-with-unused-key"}},
		RAG: RAGProperties{
			MinIO:     MinIOProperties{AccessKey: "replace-with-unused-key", SecretKey: "replace-with-unused-key"},
			Milvus:    MilvusProperties{APIKey: "replace-with-unused-key", Password: "replace-with-unused-key"},
			Embedding: EmbeddingProperties{Models: []EmbeddingModelProperties{{APIKey: "replace-with-unused-key", Enabled: &disabled}}},
		},
	}
	if err := properties.ValidateCredentials(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCredentialsRequiresEnabledServiceCredentials(t *testing.T) {
	properties := Properties{RAG: RAGProperties{MinIO: MinIOProperties{Endpoint: "storage.example.test:9000"}}}
	if err := properties.ValidateCredentials(); err == nil {
		t.Fatal("enabled MinIO requires credentials")
	}
	properties.RAG.MinIO.AccessKey = "test-storage-access"
	properties.RAG.MinIO.SecretKey = "test-storage-secret"
	properties.Sandbox.Provider = "opensandbox"
	if err := properties.ValidateCredentials(); err == nil {
		t.Fatal("enabled OpenSandbox requires credentials")
	}
	properties.Sandbox.OpenSandbox.APIKey = "test-sandbox-credential"
	if err := properties.ValidateCredentials(); err != nil {
		t.Fatal(err)
	}
}
