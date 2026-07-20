package config

import (
	"testing"
	"time"
)

func TestMapperCreatesIndependentRuntimeConfigs(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	mapper := Mapper{Now: func() time.Time { return now }}

	model := mapper.ModelConfig(ModelProperties{
		ID:      "gpt-test",
		BaseURL: "https://example.test",
		APIKey:  "secret",
	})
	if model.ID != "gpt-test" || model.Provider != "openai" || !model.CreatedAt.Equal(now) {
		t.Fatalf("unexpected model config: %#v", model)
	}

	asset := mapper.AssetConfig(AssetProperties{
		BaseURL:              "https://asset.test",
		UploadTimeoutSeconds: 30,
		TTLSeconds:           60,
	})
	if asset.Timeout != 30*time.Second || asset.DefaultTTLSeconds != 60 {
		t.Fatalf("unexpected asset config: %#v", asset)
	}

	minio := mapper.MinIOConfig(MinIOProperties{
		Endpoint:         "minio.example.test:9000",
		AccessKey:        "access-key",
		SecretKey:        "secret-key",
		SessionToken:     "session-token",
		Bucket:           "knowledge",
		Region:           "cn-east-1",
		UseSSL:           true,
		AutoCreateBucket: true,
	})
	if minio.Endpoint != "minio.example.test:9000" || minio.Bucket != "knowledge" || !minio.UseSSL || !minio.AutoCreateBucket {
		t.Fatalf("unexpected MinIO config: %#v", minio)
	}

	processor := mapper.DocumentProcessorConfig(DocumentProcessorProperties{
		PythonExecutable:      "python3",
		Script:                "./document_processor/main.py",
		Transport:             "tcp",
		WorkerCount:           4,
		MaxPendingJobs:        20,
		StartupTimeoutSeconds: 30,
		TimeoutSeconds:        90,
		ShutdownGraceSeconds:  5,
		MaxDocumentSizeMB:     128,
		ContentChunkKB:        256,
		ChunkBatchSize:        64,
		GRPCMaxMessageMB:      8,
	})
	if processor.PythonExecutable != "python3" || processor.Script != "./document_processor/main.py" || processor.Transport != "tcp" {
		t.Fatalf("unexpected document processor runtime config: %#v", processor)
	}
	if processor.WorkerCount != 4 || processor.MaxPendingJobs != 20 || processor.RequestTimeout != 90*time.Second {
		t.Fatalf("unexpected document processor config: %#v", processor)
	}
	if processor.MaxDocumentBytes != 128*1024*1024 || processor.ContentChunkBytes != 256*1024 || processor.GRPCMaxMessageBytes != 8*1024*1024 {
		t.Fatalf("unexpected document processor size config: %#v", processor)
	}

	enabled := true
	embeddingProperties := EmbeddingModelProperties{
		ID: "embedding-1", Name: "Embedding", Provider: "openai-compatible", BaseURL: "https://embedding.test/v1", APIKey: "secret", Model: "text-embedding", ModelVersion: "v1", Dimensions: 1024, MaxInputTokens: 8192, BatchSize: 16, TimeoutSeconds: 45, Enabled: &enabled,
	}
	embeddingConfig := mapper.EmbeddingProviderConfig(embeddingProperties)
	if embeddingConfig.APIKey != "secret" || embeddingConfig.Model != "text-embedding" || embeddingConfig.Dimensions != 1024 || embeddingConfig.Timeout != 45*time.Second {
		t.Fatalf("unexpected embedding provider config: %#v", embeddingConfig)
	}
	embeddingInfo := mapper.EmbeddingModelInfo(embeddingProperties)
	if embeddingInfo.ID != "embedding-1" || !embeddingInfo.Enabled || embeddingInfo.Dimensions != 1024 {
		t.Fatalf("unexpected embedding model info: %#v", embeddingInfo)
	}

	milvus := mapper.MilvusConfig(MilvusProperties{
		Address: "milvus.test:19530", Username: "user", Password: "password", Database: "knowledge", EnableTLS: true, CollectionPrefix: "vectors", Shards: 2,
	})
	if milvus.Address != "milvus.test:19530" || milvus.Database != "knowledge" || !milvus.EnableTLS || milvus.Shards != 2 {
		t.Fatalf("unexpected Milvus config: %#v", milvus)
	}
	keywordStore := mapper.KeywordStoreConfig(LocalKnowledgeProperties{MaxOpenConnections: 6}, "C:/data/knowledge.db")
	if keywordStore.Path != "C:/data/knowledge.db" || keywordStore.MaxOpenConnections != 6 {
		t.Fatalf("unexpected keyword store config: %#v", keywordStore)
	}
	vectorStore := mapper.LocalVectorStoreConfig(LocalKnowledgeProperties{MaxOpenConnections: 6}, "C:/data/knowledge-vectors.db")
	if vectorStore.Path != "C:/data/knowledge-vectors.db" || vectorStore.MaxOpenConnections != 6 {
		t.Fatalf("unexpected local vector store config: %#v", vectorStore)
	}
}

func TestMapperCreatesIndependentHookAndMCPConfigs(t *testing.T) {
	enabled := true
	mapper := Mapper{}

	hooks := HookProperties{Commands: []CommandHookProperties{{
		Event:   "session_changed",
		Command: "echo changed",
		Enabled: &enabled,
	}}}
	hookConfig := mapper.HookConfig("C:/workspace", hooks)
	if len(hookConfig.CommandHooks) != 1 || hookConfig.CommandHooks[0].Command != "echo changed" {
		t.Fatalf("unexpected hook config: %#v", hookConfig)
	}

	mcpProperties := MCPProperties{Servers: []MCPServerProperties{{
		Name:       "filesystem",
		Command:    "npx",
		Args:       []string{"-y", "server"},
		Env:        map[string]string{"MODE": "test"},
		Permission: "read",
	}}}
	mcpConfig := mapper.MCPConfig("C:/workspace", mcpProperties)
	if len(mcpConfig.Servers) != 1 || mcpConfig.Servers[0].WorkingDir != "C:/workspace" {
		t.Fatalf("unexpected mcp config: %#v", mcpConfig)
	}
	mcpConfig.Servers[0].Args[0] = "changed"
	mcpConfig.Servers[0].Env["MODE"] = "changed"
	if mcpProperties.Servers[0].Args[0] != "-y" || mcpProperties.Servers[0].Env["MODE"] != "test" {
		t.Fatal("expected MCP runtime config to own nested collections")
	}
}
