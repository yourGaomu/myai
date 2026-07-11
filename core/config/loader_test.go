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
	v.Set("redis.addr", " localhost:6379 ")
	v.Set("thread.core", 4)
	v.Set("skill.root", " custom-skills ")

	properties, err := (ViperLoader{}).Map(v, "C:/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if properties.Model.ID != "gpt-test" || properties.Model.BaseURL != "https://example.test/v1" {
		t.Fatalf("unexpected model properties: %#v", properties.Model)
	}
	if properties.Redis.Address != "localhost:6379" || properties.Thread.Core != 4 {
		t.Fatalf("unexpected infrastructure properties: %#v %#v", properties.Redis, properties.Thread)
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
