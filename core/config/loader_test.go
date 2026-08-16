package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestViperLoaderMapsAndNormalizesProperties(t *testing.T) {
	v := viper.New()
	v.Set("myai.model", " gpt-test ")
	v.Set("myai.base_url", " https://example.test/v1 ")
	v.Set("memory.extraction.model_id", " memory-extractor ")
	v.Set("memory.dream.model_id", " memory-dream ")
	v.Set("redis.addr", " localhost:6379 ")
	v.Set("rag.minio.endpoint", " https://minio.example.test:9000 ")
	v.Set("rag.id_node", 7)
	v.Set("rag.minio.access_key", " minio-access ")
	v.Set("rag.minio.secret_key", "minio-secret")
	v.Set("rag.minio.session_token", "session-token")
	v.Set("rag.minio.bucket", " knowledge ")
	v.Set("rag.minio.region", " cn-east-1 ")
	v.Set("rag.minio.use_ssl", true)
	v.Set("rag.minio.auto_create_bucket", true)
	v.Set("rag.document_processor.enabled", true)
	v.Set("rag.document_processor.python_executable", " python3 ")
	v.Set("rag.document_processor.script", " ./document_processor/main.py ")
	v.Set("rag.document_processor.transport", " tcp ")
	v.Set("rag.document_processor.worker_count", 4)
	v.Set("rag.document_processor.max_pending_jobs", 20)
	v.Set("rag.document_processor.timeout_seconds", 90)
	v.Set("rag.milvus.enabled", true)
	v.Set("rag.milvus.address", " milvus.example.test:19530 ")
	v.Set("rag.milvus.database", " knowledge ")
	v.Set("rag.milvus.collection_prefix", " vectors ")
	v.Set("rag.milvus.shards", 2)
	v.Set("rag.local.enabled", true)
	v.Set("rag.local.path", " .myai/knowledge.db ")
	v.Set("rag.local.vector_path", " .myai/knowledge-vectors.db ")
	v.Set("rag.local.max_open_connections", 6)
	v.Set("rag.retrieval.candidate_multiplier", 4)
	v.Set("rag.retrieval.max_candidates", 80)
	v.Set("rag.retrieval.min_local_results", 2)
	v.Set("rag.retrieval.min_local_score", 0.6)
	v.Set("rag.retrieval.rrf_k", 50)
	v.Set("rag.retrieval.cache_remote_results", false)
	enabled := true
	v.Set("rag.embedding.models", []map[string]any{{
		"id": " embedding-1 ", "name": " Embedding One ", "provider": " OpenAI-Compatible ",
		"base_url": " https://embedding.example.test/v1 ", "api_key": " secret ", "model": " text-embedding ",
		"model_version": " v1 ", "dimensions": 1024, "batch_size": 16, "enabled": enabled,
	}})
	v.Set("thread.core", 4)
	v.Set("subagent.worker_count", 3)
	v.Set("subagent.queue_size", 24)
	v.Set("skill.root", " custom-skills ")

	properties, err := (ViperLoader{}).Map(v, "C:/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if properties.Model.ID != "gpt-test" || properties.Model.BaseURL != "https://example.test/v1" {
		t.Fatalf("unexpected model properties: %#v", properties.Model)
	}
	if properties.Memory.Extraction.ModelID != "memory-extractor" || properties.Memory.Dream.ModelID != "memory-dream" {
		t.Fatalf("unexpected memory model properties: %#v", properties.Memory)
	}
	if properties.Redis.Address != "localhost:6379" || properties.Thread.Core != 4 {
		t.Fatalf("unexpected infrastructure properties: %#v %#v", properties.Redis, properties.Thread)
	}
	if properties.Subagent.WorkerCount != 3 || properties.Subagent.QueueSize != 24 {
		t.Fatalf("unexpected subagent properties: %#v", properties.Subagent)
	}
	if properties.Subagent.SnapshotRoot != filepath.Join("C:/workspace", DefaultSubagentSnapshotRoot) {
		t.Fatalf("unexpected subagent snapshot root: %q", properties.Subagent.SnapshotRoot)
	}
	if properties.RAG.IDNode != 7 {
		t.Fatalf("unexpected RAG snowflake node: %d", properties.RAG.IDNode)
	}
	minio := properties.RAG.MinIO
	if minio.Endpoint != "https://minio.example.test:9000" || minio.AccessKey != "minio-access" || minio.SecretKey != "minio-secret" || minio.SessionToken != "session-token" {
		t.Fatalf("unexpected MinIO connection properties: %#v", minio)
	}
	if minio.Bucket != "knowledge" || minio.Region != "cn-east-1" || !minio.UseSSL || !minio.AutoCreateBucket {
		t.Fatalf("unexpected MinIO bucket properties: %#v", minio)
	}
	processor := properties.RAG.DocumentProcessor
	if !processor.Enabled || processor.PythonExecutable != "python3" || processor.Script != "./document_processor/main.py" || processor.Transport != "tcp" {
		t.Fatalf("unexpected document processor runtime properties: %#v", processor)
	}
	if processor.WorkerCount != 4 || processor.MaxPendingJobs != 20 || processor.TimeoutSeconds != 90 {
		t.Fatalf("unexpected document processor properties: %#v", processor)
	}
	models := properties.RAG.Embedding.Models
	if len(models) != 1 || models[0].ID != "embedding-1" || models[0].Provider != "openai-compatible" || models[0].Model != "text-embedding" {
		t.Fatalf("unexpected embedding model properties: %#v", models)
	}
	if models[0].BatchSize != 16 || models[0].TimeoutSeconds != 60 || models[0].Enabled == nil || !*models[0].Enabled {
		t.Fatalf("unexpected embedding runtime defaults: %#v", models[0])
	}
	if !properties.RAG.Milvus.Enabled || properties.RAG.Milvus.Address != "milvus.example.test:19530" || properties.RAG.Milvus.Database != "knowledge" || properties.RAG.Milvus.Shards != 2 {
		t.Fatalf("unexpected Milvus properties: %#v", properties.RAG.Milvus)
	}
	if !properties.RAG.Local.Enabled || properties.RAG.Local.Path != filepath.Join("C:/workspace", ".myai/knowledge.db") || properties.RAG.Local.VectorPath != filepath.Join("C:/workspace", ".myai/knowledge-vectors.db") || properties.RAG.Local.MaxOpenConnections != 6 {
		t.Fatalf("unexpected local knowledge properties: %#v", properties.RAG.Local)
	}
	retrieval := properties.RAG.Retrieval
	if retrieval.CandidateMultiplier != 4 || retrieval.MaxCandidates != 80 || retrieval.MinLocalResults != 2 || retrieval.MinLocalScore != 0.6 || retrieval.RRFK != 50 || retrieval.CacheRemoteResults {
		t.Fatalf("unexpected retrieval properties: %#v", retrieval)
	}
	wantRoot := filepath.Join("C:/workspace", "custom-skills")
	if properties.Skill.Root != wantRoot {
		t.Fatalf("expected resolved skill root %q, got %q", wantRoot, properties.Skill.Root)
	}
}

func TestViperLoaderAppliesDefaults(t *testing.T) {
	properties, err := (ViperLoader{}).Map(viper.New(), "")
	if err != nil {
		t.Fatal(err)
	}
	if properties.Model.ID != DefaultModelID || properties.Skill.Root != DefaultSkillRoot {
		t.Fatalf("unexpected defaults: %#v", properties)
	}
	if properties.RAG.MinIO.Endpoint != "" {
		t.Fatalf("MinIO must remain disabled without an endpoint: %#v", properties.RAG.MinIO)
	}
	if properties.RAG.DocumentProcessor.TimeoutSeconds != 120 {
		t.Fatalf("unexpected document processor timeout default: %#v", properties.RAG.DocumentProcessor)
	}
	if properties.RAG.DocumentProcessor.PythonExecutable != "python" || properties.RAG.DocumentProcessor.Transport != "auto" {
		t.Fatalf("unexpected document processor runtime defaults: %#v", properties.RAG.DocumentProcessor)
	}
	if properties.RAG.Local.MaxOpenConnections != 4 {
		t.Fatalf("unexpected local knowledge defaults: %#v", properties.RAG.Local)
	}
	if properties.Subagent.WorkerCount != 2 || properties.Subagent.QueueSize != 32 {
		t.Fatalf("unexpected subagent defaults: %#v", properties.Subagent)
	}
	if properties.Subagent.SnapshotRoot != DefaultSubagentSnapshotRoot {
		t.Fatalf("unexpected subagent snapshot root default: %q", properties.Subagent.SnapshotRoot)
	}
	retrieval := properties.RAG.Retrieval
	if retrieval.CandidateMultiplier != 3 || retrieval.MaxCandidates != 100 || retrieval.MinLocalResults != 3 || retrieval.MinLocalScore != 0.55 || retrieval.RRFK != 60 || !retrieval.CacheRemoteResults {
		t.Fatalf("unexpected retrieval defaults: %#v", retrieval)
	}
}

func TestViperLoaderEnvironmentOverridesConfig(t *testing.T) {
	t.Setenv("MYAI_MODEL", "env-model")
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("myai.model", "file-model")

	properties, err := (ViperLoader{}).Map(v, "")
	if err != nil {
		t.Fatal(err)
	}
	if properties.Model.ID != "env-model" {
		t.Fatalf("expected environment model override, got %q", properties.Model.ID)
	}
}
