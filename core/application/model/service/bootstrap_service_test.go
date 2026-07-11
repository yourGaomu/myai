package service

import (
	"context"
	"errors"
	"testing"

	modelcommand "myai/core/application/model/command"
	domainmodel "myai/core/domain/model"
)

func TestBootstrapServiceLoadsConfigsAndRegistersEnabledModels(t *testing.T) {
	repo := &fakeBootstrapRepository{
		listed: []domainmodel.Config{
			validModelConfig("gpt-a"),
			{ID: "gpt-disabled", Enabled: false},
		},
	}
	registry := &fakeModelRegistry{}
	factory := &fakeModelFactory{model: fakeChatModel{}}

	result, err := (BootstrapService{
		Repository: repo,
		Registry:   registry,
		Factory:    factory,
	}).Bootstrap(context.Background(), modelcommand.Bootstrap{FallbackModelID: "gpt-a"})
	if err != nil {
		t.Fatal(err)
	}

	if result.DefaultModelID != "gpt-a" || len(result.Configs) != 2 {
		t.Fatalf("unexpected bootstrap result: %#v", result)
	}
	if registry.models["gpt-a"] == nil || registry.models["gpt-disabled"] != nil {
		t.Fatalf("expected only enabled model to be registered, got %#v", registry.models)
	}
}

func TestBootstrapServiceSeedsRepositoryWhenNoConfigsExist(t *testing.T) {
	repo := &fakeBootstrapRepository{}
	registry := &fakeModelRegistry{}

	result, err := (BootstrapService{
		Repository: repo,
		Registry:   registry,
		Factory:    &fakeModelFactory{model: fakeChatModel{}},
	}).Bootstrap(context.Background(), modelcommand.Bootstrap{
		Seed: validModelConfig("gpt-seed"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if repo.saved.ID != "gpt-seed" || result.DefaultModelID != "gpt-seed" {
		t.Fatalf("expected seed config to be saved and defaulted, repo=%#v result=%#v", repo, result)
	}
}

func TestBootstrapServiceRequiresSeedWhenRepositoryEmpty(t *testing.T) {
	_, err := (BootstrapService{
		Registry: &fakeModelRegistry{},
		Factory:  &fakeModelFactory{model: fakeChatModel{}},
	}).Bootstrap(context.Background(), modelcommand.Bootstrap{})

	if err == nil || err.Error() != "model urlConfig is empty" {
		t.Fatalf("expected missing seed error, got %v", err)
	}
}

func TestBootstrapServiceWrapsFactoryErrorsWithModelID(t *testing.T) {
	_, err := (BootstrapService{
		Registry: &fakeModelRegistry{},
		Factory:  &fakeModelFactory{err: errors.New("factory failed")},
	}).Bootstrap(context.Background(), modelcommand.Bootstrap{
		Seed: validModelConfig("gpt-seed"),
	})

	if err == nil || err.Error() != "create model gpt-seed failed: factory failed" {
		t.Fatalf("expected wrapped factory error, got %v", err)
	}
}

func TestDefaultModelID(t *testing.T) {
	models := []domainmodel.Config{
		{ID: "gpt-a", Enabled: true},
		{ID: "gpt-b", Enabled: true, IsDefault: true},
	}
	if got := DefaultModelID(models, "gpt-a"); got != "gpt-b" {
		t.Fatalf("expected explicit default, got %q", got)
	}
	models[1].IsDefault = false
	if got := DefaultModelID(models, "gpt-a"); got != "gpt-a" {
		t.Fatalf("expected fallback model, got %q", got)
	}
}

type fakeBootstrapRepository struct {
	listed []domainmodel.Config
	saved  domainmodel.Config
	err    error
}

func (r *fakeBootstrapRepository) SaveConfig(ctx context.Context, model domainmodel.Config) error {
	if r.err != nil {
		return r.err
	}
	r.saved = model
	return nil
}

func (r *fakeBootstrapRepository) ListConfigs(ctx context.Context) ([]domainmodel.Config, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.listed, nil
}
