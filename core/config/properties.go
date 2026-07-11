package config

type Properties struct {
	Model  ModelProperties
	Mongo  MongoProperties
	Redis  RedisProperties
	Thread ThreadProperties
	Asset  AssetProperties
	Skill  SkillProperties
	Hooks  HookProperties
	MCP    MCPProperties
}

type ModelProperties struct {
	ID      string
	BaseURL string
	APIKey  string
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
