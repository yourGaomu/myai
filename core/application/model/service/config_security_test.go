package service

import (
	"context"
	"strings"
	"testing"

	modelcommand "myai/core/application/model/command"
	"myai/core/domain/generation"
	domainmodel "myai/core/domain/model"
)

func TestUpdateConfigScopesRetainedKeyToSameBaseURL(t *testing.T) {
	cases := []struct {
		name, url, key string
		auth           domainmodel.AuthType
		wantError      bool
		retained       bool
	}{
		{"same endpoint", "https://example.test", "", domainmodel.AuthTypeBearer, false, true},
		{"normalized same endpoint", " https://example.test/ ", "", domainmodel.AuthTypeBearer, false, true},
		{"new host requires key", "https://other.test", "", domainmodel.AuthTypeBearer, true, false},
		{"new path requires key", "https://example.test/other", "", domainmodel.AuthTypeBearer, true, false},
		{"new host explicit key", "https://other.test", "replacement-fixture", domainmodel.AuthTypeBearer, false, false},
		{"new host no auth", "https://other.test", "", domainmodel.AuthTypeNone, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := validModelConfig("demo")
			old.Name = "demo"
			repo := &fakeConfigRepository{saved: old}
			factory := &fakeModelFactory{model: fakeChatModel{}}
			registry := &fakeModelRegistry{}
			_, err := (ConfigService{Repository: repo, Registry: registry, Factory: factory}).UpdateConfig(context.Background(), modelcommand.UpdateConfig{
				ID: old.ID, BaseURL: c.url, APIKey: c.key, AuthType: c.auth,
			})
			if c.wantError {
				if err == nil || !strings.Contains(err.Error(), "changing base url") {
					t.Fatalf("expected endpoint credential refusal, got %v", err)
				}
				if repo.saved.BaseURL != old.BaseURL || factory.config.BaseURL != "" || len(registry.infos) != 0 {
					t.Fatal("rejected credential change reached persistence or factory")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.retained {
				if factory.config.APIKey != old.APIKey {
					t.Fatal("same endpoint did not retain key")
				}
			} else if factory.config.APIKey == old.APIKey {
				t.Fatal("new endpoint inherited old key")
			}
		})
	}
}

func TestModelMetadataIsValidatedAndPreservedOnUpdate(t *testing.T) {
	repo, registry := &fakeConfigRepository{}, &fakeModelRegistry{}
	service := ConfigService{Repository: repo, Registry: registry, Factory: &fakeModelFactory{model: fakeChatModel{}}}
	result, err := service.AddConfig(context.Background(), modelcommand.AddConfig{
		ID: "demo", BaseURL: "https://example.test", APIKey: "fixture", ContextWindowTokens: 16000, MaxOutputTokens: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.ContextWindowTokens != 16000 || registry.infos["demo"].MaxOutputTokens != 4000 || registry.infos["demo"].ContextWindowTokens != 16000 {
		t.Fatal("declared model metadata did not reach registry")
	}
	updated, err := service.UpdateConfig(context.Background(), modelcommand.UpdateConfig{ID: "demo", BaseURL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Config.ContextWindowTokens != 16000 || updated.Config.MaxOutputTokens != 4000 {
		t.Fatal("omitted token metadata was not preserved")
	}
	zero := 0
	cleared, err := service.UpdateConfig(context.Background(), modelcommand.UpdateConfig{ID: "demo", BaseURL: "https://example.test", ContextWindowTokens: &zero, MaxOutputTokens: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Config.ContextWindowTokens != 0 || registry.infos["demo"].MaxOutputTokens != 0 {
		t.Fatal("explicit zero did not clear unknown metadata")
	}
	for _, limits := range []struct{ window, output, defaults int }{{-1, 0, 0}, {0, -1, 0}, {1000, 1001, 0}, {4000, 1000, 1001}, {1000, 0, 1001}} {
		config := validModelConfig("demo")
		config.Name = "demo"
		config.ContextWindowTokens, config.MaxOutputTokens = limits.window, limits.output
		if limits.defaults > 0 {
			config.DefaultGenerationSettings = generation.Settings{MaxOutputTokens: &limits.defaults}
		}
		if err := validateModelConfig(config, service.Factory); err == nil {
			t.Errorf("invalid declared limits accepted: %#v", limits)
		}
	}
}
