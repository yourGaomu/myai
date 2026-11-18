package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestHistoryPathAndModelCapacityEnvironmentMapping(t *testing.T) {
	persistent := filepath.Join(t.TempDir(), "history")
	t.Setenv("WORKSPACE_HISTORY_PATH", persistent)
	t.Setenv("MYAI_CONTEXT_WINDOW_TOKENS", "128000")
	t.Setenv("MYAI_MAX_OUTPUT_LIMIT", "16000")
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	properties, err := (ViperLoader{}).Map(v, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if properties.WorkspaceHistory.Path != persistent || properties.Model.ContextWindowTokens != 128000 || properties.Model.MaxOutputLimit != 16000 {
		t.Fatal("history path and model capacities did not use their configured environment settings")
	}
}
