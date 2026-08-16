package config

type Properties struct {
	Model    ModelProperties
	Memory   MemoryProperties
	Mongo    MongoProperties
	Redis    RedisProperties
	RAG      RAGProperties
	Thread   ThreadProperties
	Asset    AssetProperties
	Skill    SkillProperties
	Sandbox  SandboxProperties
	Subagent SubagentProperties
	Hooks    HookProperties
	MCP      MCPProperties
}

type MemoryProperties struct {
	Extraction MemoryExtractionProperties
	Dream      MemoryDreamProperties
}

type MemoryExtractionProperties struct {
	ModelID string
}

type MemoryDreamProperties struct {
	ModelID string
}

type ModelProperties struct {
	ID              string
	BaseURL         string
	APIKey          string
	Temperature     *float64
	TopP            *float64
	MaxOutputTokens *int
}

type MongoProperties struct {
	URI      string
	Database string
}

type RedisProperties struct {
	Address  string
	Password string
	DB       int
}

type RAGProperties struct {
	IDNode            int64
	MinIO             MinIOProperties
	DocumentProcessor DocumentProcessorProperties
	Embedding         EmbeddingProperties
	Milvus            MilvusProperties
	Local             LocalKnowledgeProperties
	Retrieval         RetrievalProperties
}

type RetrievalProperties struct {
	CandidateMultiplier int
	MaxCandidates       int
	MinLocalResults     int
	MinLocalScore       float64
	RRFK                int
	CacheRemoteResults  bool
}

type LocalKnowledgeProperties struct {
	Enabled            bool
	Path               string
	VectorPath         string
	MaxOpenConnections int
}

type MilvusProperties struct {
	Enabled          bool
	Address          string
	Username         string
	Password         string
	Database         string
	APIKey           string
	EnableTLS        bool
	CollectionPrefix string
	Shards           int32
}

type EmbeddingProperties struct {
	Models []EmbeddingModelProperties `mapstructure:"models"`
}

type EmbeddingModelProperties struct {
	ID                string `mapstructure:"id"`
	Name              string `mapstructure:"name"`
	Provider          string `mapstructure:"provider"`
	BaseURL           string `mapstructure:"base_url"`
	APIKey            string `mapstructure:"api_key"`
	Model             string `mapstructure:"model"`
	ModelVersion      string `mapstructure:"model_version"`
	Dimensions        int    `mapstructure:"dimensions"`
	MaxInputTokens    int    `mapstructure:"max_input_tokens"`
	BatchSize         int    `mapstructure:"batch_size"`
	TimeoutSeconds    int    `mapstructure:"timeout_seconds"`
	RequestDimensions bool   `mapstructure:"request_dimensions"`
	PreserveNewLines  bool   `mapstructure:"preserve_new_lines"`
	Enabled           *bool  `mapstructure:"enabled"`
}

type DocumentProcessorProperties struct {
	Enabled               bool
	PythonExecutable      string
	Script                string
	WorkingDirectory      string
	Transport             string
	WorkerCount           int
	MaxPendingJobs        int
	StartupTimeoutSeconds int
	TimeoutSeconds        int
	ShutdownGraceSeconds  int
	MaxDocumentSizeMB     int64
	ContentChunkKB        int
	ChunkBatchSize        int
	GRPCMaxMessageMB      int
	TempDirectory         string
}

type MinIOProperties struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	SessionToken     string
	Bucket           string
	Region           string
	UseSSL           bool
	AutoCreateBucket bool
}

type ThreadProperties struct {
	Core      int
	QueueSize int
}

type AssetProperties struct {
	BaseURL              string
	UploadTimeoutSeconds int
	TTLSeconds           int64
	MaxVisits            int64
}

type SkillProperties struct {
	Root     string
	Registry string
}

type SandboxProperties struct {
	Provider    string
	OpenSandbox OpenSandboxProperties
}

type OpenSandboxProperties struct {
	Endpoint              string
	APIKey                string
	UseServerProxy        bool
	Image                 string
	CPU                   string
	Memory                string
	SandboxTimeoutSeconds int
	CommandTimeoutSeconds int
	RequestTimeoutSeconds int
	MaxDownloadMB         int64
}

type SubagentProperties struct {
	WorkerCount  int
	QueueSize    int
	SnapshotRoot string
}

type HookProperties struct {
	Commands []CommandHookProperties
}

type MCPProperties struct {
	Servers []MCPServerProperties
}

type CommandHookProperties struct {
	Event   string `mapstructure:"event"`
	Command string `mapstructure:"command"`
	Timeout string `mapstructure:"timeout"`
	WorkDir string `mapstructure:"work_dir"`
	Enabled *bool  `mapstructure:"enabled"`
}

type MCPServerProperties struct {
	Name            string            `mapstructure:"name"`
	Command         string            `mapstructure:"command"`
	Args            []string          `mapstructure:"args"`
	Env             map[string]string `mapstructure:"env"`
	WorkingDir      string            `mapstructure:"working_dir"`
	Permission      string            `mapstructure:"permission"`
	TimeoutSeconds  int               `mapstructure:"timeout_seconds"`
	ProtocolVersion string            `mapstructure:"protocol_version"`
	Disabled        bool              `mapstructure:"disabled"`
	Required        bool              `mapstructure:"required"`
}
