package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestOutboxDefaultsAndEnvironmentMapping(t *testing.T) {
	defaults, err := (ViperLoader{}).Map(viper.New(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := defaults.Persistence.Outbox; got.Path != "" || got.MaxPending != 10000 || got.MaxEntries != 20000 || got.MaxBytes != 64*1024*1024 || got.RetainDone != 10000 {
		t.Fatalf("unexpected outbox defaults: %+v", got)
	}
	path := filepath.Join(t.TempDir(), "outbox.db")
	t.Setenv("PERSISTENCE_OUTBOX_PATH", path)
	t.Setenv("PERSISTENCE_OUTBOX_MAX_PENDING", "25")
	t.Setenv("PERSISTENCE_OUTBOX_MAX_ENTRIES", "75")
	t.Setenv("PERSISTENCE_OUTBOX_MAX_BYTES", "8388608")
	t.Setenv("PERSISTENCE_OUTBOX_RETAIN_DONE", "40")
	v := viper.New()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	properties, err := (ViperLoader{}).Map(v, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got := properties.Persistence.Outbox
	if got.Path != path || got.MaxPending != 25 || got.MaxEntries != 75 || got.MaxBytes != 8388608 || got.RetainDone != 40 {
		t.Fatalf("outbox environment overrides not applied: %+v", got)
	}
}

func TestOutboxRejectsInvalidLimits(t *testing.T) {
	for _, properties := range []OutboxProperties{
		{MaxPending: -1}, {MaxEntries: -1}, {MaxBytes: -1}, {RetainDone: -1}, {MaxBytes: 1024},
		{MaxPending: 100, MaxEntries: 50, RetainDone: 10},
		{MaxPending: 10, MaxEntries: 50, RetainDone: 100},
	} {
		if err := normalizeOutboxProperties(&properties); err == nil {
			t.Fatalf("invalid capacity was accepted: %+v", properties)
		}
	}
}
