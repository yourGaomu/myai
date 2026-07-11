package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	DefaultConfigFile = "./resource/application.yaml"
	DefaultModelID    = "gpt-5.5"
	DefaultSkillRoot  = "skills"
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

	properties := Properties{
		Model: ModelProperties{
			ID:      strings.TrimSpace(v.GetString("myai.model")),
			BaseURL: strings.TrimSpace(v.GetString("myai.base_url")),
			APIKey:  strings.TrimSpace(v.GetString("myai.api_key")),
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
	}
	if properties.Model.ID == "" {
		properties.Model.ID = DefaultModelID
	}
	if properties.Skill.Root == "" {
		properties.Skill.Root = DefaultSkillRoot
	}
	properties.Skill.Root = resolveWorkspacePath(workspace, properties.Skill.Root)

	if err := v.UnmarshalKey("hooks.commands", &properties.Hooks.Commands); err != nil {
		return Properties{}, err
	}
	if err := v.UnmarshalKey("mcp.servers", &properties.MCP.Servers); err != nil {
		return Properties{}, err
	}
	return properties, nil
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
