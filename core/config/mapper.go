package config

import (
	"time"

	grpcprocessor "myai/core/adapter/documentprocessor/grpc"
	openaiembedding "myai/core/adapter/embedding/openaicompatible"
	sqlitefts5 "myai/core/adapter/keywordstore/sqlitefts5"
	minioadapter "myai/core/adapter/objectstorage/minio"
	opensandboxadapter "myai/core/adapter/sandbox/opensandbox"
	milvusadapter "myai/core/adapter/vectorstore/milvus"
	sqlitevec "myai/core/adapter/vectorstore/sqlitevec"
	"myai/core/asset"
	generation "myai/core/domain/generation"
	domainknowledge "myai/core/domain/knowledge"
	domainmodel "myai/core/domain/model"
	"myai/core/hook"
	"myai/core/mcp"
)

type Mapper struct {
	Now func() time.Time
}

func (Mapper) EmbeddingProviderConfig(properties EmbeddingModelProperties) openaiembedding.Config {
	return openaiembedding.Config{
		APIKey:            properties.APIKey,
		BaseURL:           properties.BaseURL,
		Model:             properties.Model,
		Dimensions:        properties.Dimensions,
		BatchSize:         properties.BatchSize,
		Timeout:           time.Duration(properties.TimeoutSeconds) * time.Second,
		RequestDimensions: properties.RequestDimensions,
		PreserveNewLines:  properties.PreserveNewLines,
	}
}

func (Mapper) EmbeddingModelInfo(properties EmbeddingModelProperties) domainknowledge.EmbeddingModelInfo {
	enabled := true
	if properties.Enabled != nil {
		enabled = *properties.Enabled
	}
	return domainknowledge.EmbeddingModelInfo{
		ID:             properties.ID,
		Name:           properties.Name,
		Provider:       properties.Provider,
		Model:          properties.Model,
		ModelVersion:   properties.ModelVersion,
		Dimensions:     properties.Dimensions,
		MaxInputTokens: properties.MaxInputTokens,
		Enabled:        enabled,
	}
}

func (Mapper) MilvusConfig(properties MilvusProperties) milvusadapter.Config {
	return milvusadapter.Config{
		Address:          properties.Address,
		Username:         properties.Username,
		Password:         properties.Password,
		Database:         properties.Database,
		APIKey:           properties.APIKey,
		EnableTLS:        properties.EnableTLS,
		CollectionPrefix: properties.CollectionPrefix,
		Shards:           properties.Shards,
	}
}

func (Mapper) KeywordStoreConfig(properties LocalKnowledgeProperties, path string) sqlitefts5.Config {
	return sqlitefts5.Config{
		Path:               path,
		MaxOpenConnections: properties.MaxOpenConnections,
	}
}

func (Mapper) LocalVectorStoreConfig(properties LocalKnowledgeProperties, path string) sqlitevec.Config {
	return sqlitevec.Config{
		Path:               path,
		MaxOpenConnections: properties.MaxOpenConnections,
	}
}

func (m Mapper) ModelConfig(properties ModelProperties) domainmodel.Config {
	now := m.now()
	return domainmodel.Config{
		ID:        properties.ID,
		Name:      properties.ID,
		Provider:  "openai",
		BaseURL:   properties.BaseURL,
		APIKey:    properties.APIKey,
		ModelName: properties.ID,
		Enabled:   true,
		IsDefault: true,
		DefaultGenerationSettings: generation.Settings{
			Temperature:     cloneFloat64(properties.Temperature),
			TopP:            cloneFloat64(properties.TopP),
			MaxOutputTokens: cloneInt(properties.MaxOutputTokens),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (Mapper) AssetConfig(properties AssetProperties) asset.Config {
	return asset.Config{
		BaseURL:           properties.BaseURL,
		Timeout:           time.Duration(properties.UploadTimeoutSeconds) * time.Second,
		DefaultTTLSeconds: properties.TTLSeconds,
		DefaultMaxVisits:  properties.MaxVisits,
	}
}

func (Mapper) MinIOConfig(properties MinIOProperties) minioadapter.Config {
	return minioadapter.Config{
		Endpoint:         properties.Endpoint,
		AccessKey:        properties.AccessKey,
		SecretKey:        properties.SecretKey,
		SessionToken:     properties.SessionToken,
		Bucket:           properties.Bucket,
		Region:           properties.Region,
		UseSSL:           properties.UseSSL,
		AutoCreateBucket: properties.AutoCreateBucket,
	}
}

func (Mapper) DocumentProcessorConfig(properties DocumentProcessorProperties) grpcprocessor.Config {
	return grpcprocessor.Config{
		PythonExecutable:    properties.PythonExecutable,
		Script:              properties.Script,
		WorkingDir:          properties.WorkingDirectory,
		Transport:           properties.Transport,
		WorkerCount:         properties.WorkerCount,
		MaxPendingJobs:      properties.MaxPendingJobs,
		StartupTimeout:      time.Duration(properties.StartupTimeoutSeconds) * time.Second,
		RequestTimeout:      time.Duration(properties.TimeoutSeconds) * time.Second,
		ShutdownGrace:       time.Duration(properties.ShutdownGraceSeconds) * time.Second,
		MaxDocumentBytes:    properties.MaxDocumentSizeMB * 1024 * 1024,
		ContentChunkBytes:   properties.ContentChunkKB * 1024,
		ChunkBatchSize:      properties.ChunkBatchSize,
		GRPCMaxMessageBytes: properties.GRPCMaxMessageMB * 1024 * 1024,
		TempDir:             properties.TempDirectory,
	}
}

func (Mapper) OpenSandboxConfig(properties OpenSandboxProperties) opensandboxadapter.Config {
	return opensandboxadapter.Config{
		Endpoint:         properties.Endpoint,
		APIKey:           properties.APIKey,
		UseServerProxy:   properties.UseServerProxy,
		Image:            properties.Image,
		CPU:              properties.CPU,
		Memory:           properties.Memory,
		SandboxTimeout:   time.Duration(properties.SandboxTimeoutSeconds) * time.Second,
		CommandTimeout:   time.Duration(properties.CommandTimeoutSeconds) * time.Second,
		RequestTimeout:   time.Duration(properties.RequestTimeoutSeconds) * time.Second,
		MaxDownloadBytes: properties.MaxDownloadMB * 1024 * 1024,
	}
}

func (Mapper) HookConfig(workspace string, properties HookProperties) hook.Config {
	commands := make([]hook.CommandHookConfig, 0, len(properties.Commands))
	for _, command := range properties.Commands {
		commands = append(commands, hook.CommandHookConfig{
			Event:   command.Event,
			Command: command.Command,
			Timeout: command.Timeout,
			WorkDir: command.WorkDir,
			Enabled: command.Enabled,
		})
	}
	return hook.Config{
		Workspace:    workspace,
		CommandHooks: commands,
	}
}

func (Mapper) MCPConfig(workspace string, properties MCPProperties) mcp.Config {
	servers := make([]mcp.ServerConfig, 0, len(properties.Servers))
	for _, server := range properties.Servers {
		servers = append(servers, mcp.ServerConfig{
			Name:            server.Name,
			Command:         server.Command,
			Args:            append([]string(nil), server.Args...),
			Env:             cloneStringMap(server.Env),
			WorkingDir:      server.WorkingDir,
			Permission:      server.Permission,
			TimeoutSeconds:  server.TimeoutSeconds,
			ProtocolVersion: server.ProtocolVersion,
			Disabled:        server.Disabled,
			Required:        server.Required,
		})
	}
	return mcp.NormalizeConfig(mcp.Config{Servers: servers}, workspace)
}

func (m Mapper) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
