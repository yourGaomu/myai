package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domain "myai/core/domain/imagemodel"
)

func TestImageModelConfigProtocolValidation(t *testing.T) {
	valid := func(kind MessageType, payload any) Message {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return Message{Type: kind, RequestID: "image-model-request", Payload: raw}
	}
	cases := []struct {
		name    string
		message Message
		wantErr bool
	}{
		{"list", Message{Type: TypeImageModelList, RequestID: "list"}, false},
		{"add", valid(TypeImageModelConfigAdd, ImageModelConfigAddPayload{ID: "image-1", Protocol: string(domain.ProtocolOpenAIImages), ModelName: "model"}), false},
		{"test", valid(TypeImageModelConfigTest, ImageModelConfigTestPayload{Protocol: string(domain.ProtocolGeminiNativeImage), ModelName: "model"}), false},
		{"update", valid(TypeImageModelConfigUpdate, ImageModelConfigUpdatePayload{ID: "image-1", Protocol: string(domain.ProtocolGoogleImagen), ModelName: "imagen"}), false},
		{"delete", valid(TypeImageModelConfigDelete, ImageModelConfigDeletePayload{ID: "image-1"}), false},
		{"enable", valid(TypeImageModelConfigEnabledSet, ImageModelConfigEnabledSetPayload{ID: "image-1", Enabled: true}), false},
		{"default", valid(TypeImageModelConfigDefaultSet, ImageModelConfigDefaultSetPayload{ID: "image-1"}), false},
		{"unknown protocol", valid(TypeImageModelConfigAdd, ImageModelConfigAddPayload{ID: "image-1", Protocol: "unknown", ModelName: "model"}), true},
		{"missing request id", Message{Type: TypeImageModelList}, true},
		{"oversized payload", Message{Type: TypeImageModelList, RequestID: "large", Payload: []byte(strings.Repeat("x", 64*1024+1))}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateImageModelConfigMessage(tc.message)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestImageModelSummaryWireDTOOmitsSecretsAndPreservesSchema(t *testing.T) {
	summary := NewImageModelSummary(domain.Summary{ID: "image-1", Name: "Generator", Protocol: domain.ProtocolOpenAIImages, AuthType: domain.AuthTypeBearer, BaseURL: "https://images.example.test", ModelName: "image-model", HasAPIKey: true, Enabled: true, IsDefault: true, SupportsReferenceImages: true, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	raw, err := json.Marshal(ImageModelConfigMutationResultPayload{Model: &summary, Models: []ImageModelSummary{summary}, DefaultModelID: summary.ID})
	if err != nil {
		t.Fatal(err)
	}
	value := string(raw)
	for _, field := range []string{`"id":"image-1"`, `"has_api_key":true`, `"supports_reference_images":true`, `"default_model_id":"image-1"`} {
		if !strings.Contains(value, field) {
			t.Fatalf("missing %s in %s", field, value)
		}
	}
	for _, secret := range []string{`"api_key":`, `"secret"`, `"token"`} {
		if strings.Contains(value, secret) {
			t.Fatalf("wire DTO exposed %q: %s", secret, value)
		}
	}
}
