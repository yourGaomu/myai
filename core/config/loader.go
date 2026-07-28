package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	DefaultConfigFile           = "./resource/application.yaml"
	DefaultModelID              = "gpt-5.5"
	DefaultSkillRoot            = "skills"
	DefaultSubagentSnapshotRoot = ".myai/subagent-snapshots"
)

type ViperLoader struct {
	ConfigFile string
}

func (l ViperLoader) Load(workspace string) (Properties, error) {
	// YAML 提供默认值，环境变量覆盖部署差异；workspace 用于解析 Skill 等相对路径。
	v := viper.New()
	v.SetConfigFile(l.configFile())
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Properties{}, err
	}
	return l.Map(v, workspace)
}

func (l ViperLoader) LoadOptional(workspace string) (Properties, bool, error) {
	v := viper.New()
	v.SetConfigFile(l.configFile())
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || os.IsNotExist(err) {
			return Properties{}, false, nil
		}
		return Properties{}, false, err
	}
	properties, err := l.Map(v, workspace)
	return properties, true, err
}

func (l ViperLoader) Map(v *viper.Viper, workspace string) (Properties, error) {
	if v == nil {
		return Properties{}, errors.New("config source is nil")
	}

	cacheRemoteResults := true
	if v.IsSet("rag.retrieval.cache_remote_results") {
		cacheRemoteResults = v.GetBool("rag.retrieval.cache_remote_results")
	}
	useSandboxServerProxy := true
	if v.IsSet("sandbox.opensandbox.use_server_proxy") {
		useSandboxServerProxy = v.GetBool("sandbox.opensandbox.use_server_proxy")
	}
	properties := Properties{
		Model: ModelProperties{
			ID:              strings.TrimSpace(v.GetString("myai.model")),
			BaseURL:         strings.TrimSpace(v.GetString("myai.base_url")),
			APIKey:          strings.TrimSpace(v.GetString("myai.api_key")),
			Temperature:     optionalFloat64(v, "myai.temperature"),
			TopP:            optionalFloat64(v, "myai.top_p"),
			MaxOutputTokens: optionalInt(v, "myai.max_output_tokens"),
		},
		Mongo: MongoProperties{
			URI:      strings.TrimSpace(v.GetString("mongo.uri")),
			Database: strings.TrimSpace(v.GetString("mongo.database")),
		},
		Redis: RedisProperties{
			Address:  strings.TrimSpace(v.GetString("redis.addr")),
			Password: v.GetString("redis.password"),
			DB:       v.GetInt("redis.db"),
		},
		RAG: RAGProperties{
			IDNode: v.GetInt64("rag.id_node"),
			Retrieval: RetrievalProperties{
				CandidateMultiplier: v.GetInt("rag.retrieval.candidate_multiplier"),
				MaxCandidates:       v.GetInt("rag.retrieval.max_candidates"),
				MinLocalResults:     v.GetInt("rag.retrieval.min_local_results"),
				MinLocalScore:       v.GetFloat64("rag.retrieval.min_local_score"),
				RRFK:                v.GetInt("rag.retrieval.rrf_k"),
				CacheRemoteResults:  cacheRemoteResults,
			},
			Local: LocalKnowledgeProperties{
				Enabled:            v.GetBool("rag.local.enabled"),
				Path:               strings.TrimSpace(v.GetString("rag.local.path")),
				VectorPath:         strings.TrimSpace(v.GetString("rag.local.vector_path")),
				MaxOpenConnections: v.GetInt("rag.local.max_open_connections"),
			},
			Milvus: MilvusProperties{
				Enabled:          v.GetBool("rag.milvus.enabled"),
				Address:          strings.TrimSpace(v.GetString("rag.milvus.address")),
				Username:         strings.TrimSpace(v.GetString("rag.milvus.username")),
				Password:         v.GetString("rag.milvus.password"),
				Database:         strings.TrimSpace(v.GetString("rag.milvus.database")),
				APIKey:           strings.TrimSpace(v.GetString("rag.milvus.api_key")),
				EnableTLS:        v.GetBool("rag.milvus.enable_tls"),
				CollectionPrefix: strings.TrimSpace(v.GetString("rag.milvus.collection_prefix")),
				Shards:           int32(v.GetInt("rag.milvus.shards")),
			},
			MinIO: MinIOProperties{
				Endpoint:         strings.TrimSpace(v.GetString("rag.minio.endpoint")),
				AccessKey:        strings.TrimSpace(v.GetString("rag.minio.access_key")),
				SecretKey:        v.GetString("rag.minio.secret_key"),
				SessionToken:     v.GetString("rag.minio.session_token"),
				Bucket:           strings.TrimSpace(v.GetString("rag.minio.bucket")),
				Region:           strings.TrimSpace(v.GetString("rag.minio.region")),
				UseSSL:           v.GetBool("rag.minio.use_ssl"),
				AutoCreateBucket: v.GetBool("rag.minio.auto_create_bucket"),
			},
			DocumentProcessor: DocumentProcessorProperties{
				Enabled:               v.GetBool("rag.document_processor.enabled"),
				PythonExecutable:      strings.TrimSpace(v.GetString("rag.document_processor.python_executable")),
				Script:                strings.TrimSpace(v.GetString("rag.document_processor.script")),
				WorkingDirectory:      strings.TrimSpace(v.GetString("rag.document_processor.working_directory")),
				Transport:             strings.TrimSpace(v.GetString("rag.document_processor.transport")),
				WorkerCount:           v.GetInt("rag.document_processor.worker_count"),
				MaxPendingJobs:        v.GetInt("rag.document_processor.max_pending_jobs"),
				StartupTimeoutSeconds: v.GetInt("rag.document_processor.startup_timeout_seconds"),
				TimeoutSeconds:        v.GetInt("rag.document_processor.timeout_seconds"),
				ShutdownGraceSeconds:  v.GetInt("rag.document_processor.shutdown_grace_seconds"),
				MaxDocumentSizeMB:     v.GetInt64("rag.document_processor.max_document_size_mb"),
				ContentChunkKB:        v.GetInt("rag.document_processor.content_chunk_kb"),
				ChunkBatchSize:        v.GetInt("rag.document_processor.chunk_batch_size"),
				GRPCMaxMessageMB:      v.GetInt("rag.document_processor.grpc_max_message_mb"),
				TempDirectory:         strings.TrimSpace(v.GetString("rag.document_processor.temp_directory")),
			},
		},
		Thread: ThreadProperties{
			Core:      v.GetInt("thread.core"),
			QueueSize: v.GetInt("thread.queueSize"),
		},
		Asset: AssetProperties{
			BaseURL:              strings.TrimSpace(v.GetString("asset.shortener_base_url")),
			UploadTimeoutSeconds: v.GetInt("asset.upload_timeout_seconds"),
			TTLSeconds:           v.GetInt64("asset.ttl_seconds"),
			MaxVisits:            v.GetInt64("asset.max_visits"),
		},
		Skill: SkillProperties{
			Root:     strings.TrimSpace(v.GetString("skill.root")),
			Registry: strings.TrimSpace(v.GetString("skill.registry")),
		},
		Sandbox: SandboxProperties{
			Provider: strings.ToLower(strings.TrimSpace(v.GetString("sandbox.provider"))),
			OpenSandbox: OpenSandboxProperties{
				Endpoint:              strings.TrimSpace(v.GetString("sandbox.opensandbox.endpoint")),
				APIKey:                strings.TrimSpace(v.GetString("sandbox.opensandbox.api_key")),
				UseServerProxy:        useSandboxServerProxy,
				Image:                 strings.TrimSpace(v.GetString("sandbox.opensandbox.image")),
				CPU:                   strings.TrimSpace(v.GetString("sandbox.opensandbox.cpu")),
				Memory:                strings.TrimSpace(v.GetString("sandbox.opensandbox.memory")),
				SandboxTimeoutSeconds: v.GetInt("sandbox.opensandbox.sandbox_timeout_seconds"),
				CommandTimeoutSeconds: v.GetInt("sandbox.opensandbox.command_timeout_seconds"),
				RequestTimeoutSeconds: v.GetInt("sandbox.opensandbox.request_timeout_seconds"),
				MaxDownloadMB:         v.GetInt64("sandbox.opensandbox.max_download_mb"),
			},
		},
		Subagent: SubagentProperties{
			WorkerCount:  v.GetInt("subagent.worker_count"),
			QueueSize:    v.GetInt("subagent.queue_size"),
			SnapshotRoot: strings.TrimSpace(v.GetString("subagent.snapshot_root")),
		},
	}
	if properties.Model.ID == "" {
		properties.Model.ID = DefaultModelID
	}
	if properties.Skill.Root == "" {
		properties.Skill.Root = DefaultSkillRoot
	}
	if properties.Sandbox.Provider == "" {
		properties.Sandbox.Provider = "local"
	}
	if properties.Sandbox.OpenSandbox.Endpoint == "" {
		properties.Sandbox.OpenSandbox.Endpoint = "http://127.0.0.1:8090"
	}
	if properties.Sandbox.OpenSandbox.Image == "" {
		properties.Sandbox.OpenSandbox.Image = "python:3.12-slim"
	}
	if properties.Sandbox.OpenSandbox.CPU == "" {
		properties.Sandbox.OpenSandbox.CPU = "500m"
	}
	if properties.Sandbox.OpenSandbox.Memory == "" {
		properties.Sandbox.OpenSandbox.Memory = "512Mi"
	}
	if properties.Sandbox.OpenSandbox.SandboxTimeoutSeconds == 0 {
		properties.Sandbox.OpenSandbox.SandboxTimeoutSeconds = 300
	}
	if properties.Sandbox.OpenSandbox.CommandTimeoutSeconds == 0 {
		properties.Sandbox.OpenSandbox.CommandTimeoutSeconds = 60
	}
	if properties.Sandbox.OpenSandbox.RequestTimeoutSeconds == 0 {
		properties.Sandbox.OpenSandbox.RequestTimeoutSeconds = 30
	}
	if properties.Sandbox.OpenSandbox.MaxDownloadMB == 0 {
		properties.Sandbox.OpenSandbox.MaxDownloadMB = 16
	}
	if properties.Subagent.WorkerCount <= 0 {
		properties.Subagent.WorkerCount = 2
	}
	if properties.Subagent.QueueSize <= 0 {
		properties.Subagent.QueueSize = 32
	}
	if properties.Subagent.SnapshotRoot == "" {
		properties.Subagent.SnapshotRoot = DefaultSubagentSnapshotRoot
	}
	if properties.RAG.DocumentProcessor.TimeoutSeconds == 0 {
		properties.RAG.DocumentProcessor.TimeoutSeconds = 120
	}
	if properties.RAG.DocumentProcessor.StartupTimeoutSeconds == 0 {
		properties.RAG.DocumentProcessor.StartupTimeoutSeconds = 30
	}
	if properties.RAG.DocumentProcessor.ShutdownGraceSeconds == 0 {
		properties.RAG.DocumentProcessor.ShutdownGraceSeconds = 5
	}
	if properties.RAG.DocumentProcessor.PythonExecutable == "" {
		properties.RAG.DocumentProcessor.PythonExecutable = "python"
	}
	if properties.RAG.DocumentProcessor.Script == "" {
		properties.RAG.DocumentProcessor.Script = "./document_processor/main.py"
	}
	if properties.RAG.DocumentProcessor.Transport == "" {
		properties.RAG.DocumentProcessor.Transport = "auto"
	}
	if properties.RAG.Local.MaxOpenConnections == 0 {
		properties.RAG.Local.MaxOpenConnections = 4
	}
	if properties.RAG.Retrieval.CandidateMultiplier == 0 {
		properties.RAG.Retrieval.CandidateMultiplier = 3
	}
	if properties.RAG.Retrieval.MaxCandidates == 0 {
		properties.RAG.Retrieval.MaxCandidates = 100
	}
	if properties.RAG.Retrieval.MinLocalResults == 0 {
		properties.RAG.Retrieval.MinLocalResults = 3
	}
	if properties.RAG.Retrieval.MinLocalScore == 0 {
		properties.RAG.Retrieval.MinLocalScore = 0.55
	}
	if properties.RAG.Retrieval.RRFK == 0 {
		properties.RAG.Retrieval.RRFK = 60
	}
	if properties.RAG.Local.Path != "" {
		properties.RAG.Local.Path = resolveWorkspacePath(workspace, properties.RAG.Local.Path)
	}
	if properties.RAG.Local.VectorPath != "" {
		properties.RAG.Local.VectorPath = resolveWorkspacePath(workspace, properties.RAG.Local.VectorPath)
	}
	if err := v.UnmarshalKey("rag.embedding", &properties.RAG.Embedding); err != nil {
		return Properties{}, err
	}
	for index := range properties.RAG.Embedding.Models {
		model := &properties.RAG.Embedding.Models[index]
		model.ID = strings.TrimSpace(model.ID)
		model.Name = strings.TrimSpace(model.Name)
		model.Provider = strings.ToLower(strings.TrimSpace(model.Provider))
		model.BaseURL = strings.TrimSpace(model.BaseURL)
		model.APIKey = strings.TrimSpace(model.APIKey)
		model.Model = strings.TrimSpace(model.Model)
		model.ModelVersion = strings.TrimSpace(model.ModelVersion)
		if model.Name == "" {
			model.Name = model.ID
		}
		if model.Provider == "" {
			model.Provider = "openai-compatible"
		}
		if model.BatchSize == 0 {
			model.BatchSize = 32
		}
		if model.TimeoutSeconds == 0 {
			model.TimeoutSeconds = 60
		}
	}
	properties.Skill.Root = resolveWorkspacePath(workspace, properties.Skill.Root)
	properties.Subagent.SnapshotRoot = resolveWorkspacePath(workspace, properties.Subagent.SnapshotRoot)

	if err := v.UnmarshalKey("hooks.commands", &properties.Hooks.Commands); err != nil {
		return Properties{}, err
	}
	if err := v.UnmarshalKey("mcp.servers", &properties.MCP.Servers); err != nil {
		return Properties{}, err
	}
	return properties, nil
}

func optionalFloat64(v *viper.Viper, key string) *float64 {
	if !v.IsSet(key) {
		return nil
	}
	value := v.GetFloat64(key)
	return &value
}

func optionalInt(v *viper.Viper, key string) *int {
	if !v.IsSet(key) {
		return nil
	}
	value := v.GetInt(key)
	return &value
}

func (l ViperLoader) configFile() string {
	if file := strings.TrimSpace(l.ConfigFile); file != "" {
		return file
	}
	return DefaultConfigFile
}

func resolveWorkspacePath(workspace string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return path
	}
	return filepath.Join(workspace, path)
}
