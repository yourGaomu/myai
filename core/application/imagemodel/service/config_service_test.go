package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	registryadapter "myai/core/adapter/imagemodel"
	memory "myai/core/adapter/persistence/memory/imagemodel"
	domain "myai/core/domain/imagemodel"
	port "myai/core/port/imagemodel"
)

type fakeFactory struct {
	failID string
	calls  int
}

func (f *fakeFactory) Build(config domain.Config) (port.Generator, error) {
	f.calls++
	if config.ID == f.failID {
		return nil, errors.New("test build error")
	}
	return fakeGenerator{}, nil
}

type fakeGenerator struct{}

func (fakeGenerator) SupportsReferenceImages() bool { return true }
func (fakeGenerator) Generate(_ context.Context, request port.GenerateRequest) (port.GenerateResult, error) {
	var buffer bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 20, G: 30, B: 40, A: 255})
	if err := png.Encode(&buffer, img); err != nil {
		return port.GenerateResult{}, err
	}
	return port.GenerateResult{Data: buffer.Bytes(), MIMEType: "image/png"}, nil
}

type failSaveRepository struct {
	*memory.Repository
	failID string
}

func (r failSaveRepository) Save(ctx context.Context, config domain.Config) error {
	if config.ID == r.failID {
		return errors.New("write failed")
	}
	return r.Repository.Save(ctx, config)
}
func newTestConfigService(repository port.Repository, factory *fakeFactory) (*ConfigService, *registryadapter.Registry) {
	registry := registryadapter.NewRegistry()
	return &ConfigService{Repository: repository, Registry: registry, Factory: factory, Now: func() time.Time { return time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC) }}, registry
}
func imageConfig(id, key string) domain.Config {
	return domain.Config{ID: id, Name: id, Protocol: domain.ProtocolOpenAIImages, AuthType: domain.AuthTypeBearer, BaseURL: "https://images.example.test", APIKey: key, ModelName: "image-model", Enabled: true}
}

type invalidImageFactory struct{}

func (invalidImageFactory) Build(domain.Config) (port.Generator, error) {
	return invalidImageGenerator{}, nil
}

type invalidImageGenerator struct{}

func (invalidImageGenerator) SupportsReferenceImages() bool { return false }
func (invalidImageGenerator) Generate(context.Context, port.GenerateRequest) (port.GenerateResult, error) {
	return port.GenerateResult{Data: []byte("not an image"), MIMEType: "image/png"}, nil
}

func TestConfigServiceCanTestUnsavedImageModelDraft(t *testing.T) {
	service, _ := newTestConfigService(memory.NewRepository(), &fakeFactory{})
	config := imageConfig("", "draft-secret")
	result, err := service.Test(context.Background(), config)
	if err != nil || !result.OK || result.MIMEType != "image/png" {
		t.Fatalf("test result=%+v err=%v", result, err)
	}
	models, _, err := service.List(context.Background())
	if err != nil || len(models) != 0 {
		t.Fatalf("testing draft persisted a model: models=%+v err=%v", models, err)
	}
}

func TestConfigServiceAllowsGoogleADCWithoutAPIKey(t *testing.T) {
	repo := memory.NewRepository()
	service, _ := newTestConfigService(repo, &fakeFactory{})
	config := domain.Config{ID: "vertex-imagen", Protocol: domain.ProtocolGoogleImagen, AuthType: domain.AuthTypeGoogleADC, BaseURL: "https://us-central1-aiplatform.googleapis.com/v1/projects/demo/locations/us-central1", ModelName: "imagen-4.0-generate-001", Enabled: true}
	if _, err := service.Add(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.Get(context.Background(), config.ID)
	if err != nil || saved.AuthType != domain.AuthTypeGoogleADC || saved.APIKey != "" {
		t.Fatalf("saved ADC configuration=%+v err=%v", saved, err)
	}
	config.Name = "Updated Vertex model"
	config.BaseURL = "https://europe-west4-aiplatform.googleapis.com/v1/projects/demo/locations/europe-west4"
	if _, err := service.Update(context.Background(), config); err != nil {
		t.Fatalf("ADC endpoint update should not require an API key: %v", err)
	}
}

func TestConfigServiceTestRejectsInvalidImageBytes(t *testing.T) {
	repo := memory.NewRepository()
	service, _ := newTestConfigService(repo, &fakeFactory{})
	service.Factory = invalidImageFactory{}
	_, err := service.Test(context.Background(), imageConfig("image", "secret"))
	if err == nil || !strings.Contains(err.Error(), "invalid test image") {
		t.Fatalf("expected invalid image response rejection, got %v", err)
	}
}

func TestConfigServiceCRUDNeverReturnsAPIKeyAndKeepsDefaultUnique(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	factory := &fakeFactory{}
	service, registry := newTestConfigService(repo, factory)
	added, err := service.Add(ctx, imageConfig("first", "first-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if added.Model == nil || added.Model.ID != "first" || added.DefaultModelID != "" || added.Model.HasAPIKey != true {
		t.Fatalf("add result %+v", added)
	}
	if strings.Contains(mustJSON(t, added), "first-secret") {
		t.Fatal("DTO leaked API key")
	}
	if _, _, ok := registry.Get("first"); !ok {
		t.Fatal("added generator missing")
	}
	first := imageConfig("first", "")
	first.BaseURL = "https://images.example.test/v2"
	if _, err := service.Update(ctx, first); err == nil || err.Error() != "api key must be supplied when changing base url" {
		t.Fatalf("expected changed URL key guard, got %v", err)
	}
	first = imageConfig("first", "")
	first.Name = "first renamed"
	if _, err := service.Update(ctx, first); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Get(ctx, "first")
	if err != nil || stored.APIKey != "first-secret" || stored.Name != "first renamed" {
		t.Fatalf("key was not retained: %+v %v", stored, err)
	}
	second, err := service.Add(ctx, imageConfig("second", "second-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetDefault(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetEnabled(ctx, "second", false); err == nil {
		t.Fatal("default model was disabled")
	}
	models, defaultID, err := service.List(ctx)
	if err != nil || defaultID != "second" || len(models) != 2 {
		t.Fatalf("models/default %v %+v %v", defaultID, models, err)
	}
	_ = second
	if _, err := service.Delete(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := registry.Get("first"); ok {
		t.Fatal("deleted generator remained callable")
	}
}
func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestConfigServiceDoesNotPublishPartialPersistenceOnFailure(t *testing.T) {
	ctx := context.Background()
	base := memory.NewRepository()
	factory := &fakeFactory{}
	service, registry := newTestConfigService(base, factory)
	if _, err := service.Add(ctx, imageConfig("stable", "secret")); err != nil {
		t.Fatal(err)
	}
	service.Repository = failSaveRepository{Repository: base, failID: "broken"}
	if _, err := service.Add(ctx, imageConfig("broken", "other-secret")); err == nil {
		t.Fatal("expected repository write failure")
	}
	if _, _, ok := registry.Get("broken"); ok {
		t.Fatal("failed model became callable")
	}
	if _, err := base.Get(ctx, "broken"); !errors.Is(err, errors.New("image model not found")) && err == nil {
		t.Fatal("failed model persisted")
	}
	stable, err := base.Get(ctx, "stable")
	if err != nil || stable.APIKey != "secret" {
		t.Fatalf("rollback corrupted stable config: %+v %v", stable, err)
	}
}

func TestConfigServiceTestReusesSavedKeyOnlyForUnchangedURL(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewRepository()
	factory := &fakeFactory{}
	service, _ := newTestConfigService(repo, factory)
	if _, err := service.Add(ctx, imageConfig("saved", "secret")); err != nil {
		t.Fatal(err)
	}
	result, err := service.Test(ctx, domain.Config{ID: "saved", Protocol: domain.ProtocolOpenAIImages, AuthType: domain.AuthTypeBearer, BaseURL: "https://images.example.test", ModelName: "image-model"})
	if err != nil || !result.OK || result.MIMEType != "image/png" {
		t.Fatalf("reused key test result %+v err=%v", result, err)
	}
	if _, err := service.Test(ctx, domain.Config{ID: "saved", Protocol: domain.ProtocolOpenAIImages, AuthType: domain.AuthTypeBearer, BaseURL: "https://changed.example.test", ModelName: "image-model"}); err == nil {
		t.Fatal("test reused key for changed endpoint")
	}
}
