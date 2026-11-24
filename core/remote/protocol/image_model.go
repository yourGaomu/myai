package protocol

import (
	"errors"
	"strings"
	"time"

	domain "myai/core/domain/imagemodel"
)

type ImageModelSummary struct {
	ID                      string          `json:"id"`
	Name                    string          `json:"name"`
	Provider                string          `json:"provider"`
	Protocol                domain.Protocol `json:"protocol"`
	AuthType                domain.AuthType `json:"auth_type"`
	BaseURL                 string          `json:"base_url"`
	ModelName               string          `json:"model_name"`
	HasAPIKey               bool            `json:"has_api_key"`
	Enabled                 bool            `json:"enabled"`
	IsDefault               bool            `json:"is_default"`
	SupportsReferenceImages bool            `json:"supports_reference_images"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

func NewImageModelSummary(model domain.Summary) ImageModelSummary {
	return ImageModelSummary{ID: model.ID, Name: model.Name, Provider: model.Provider, Protocol: model.Protocol, AuthType: model.AuthType, BaseURL: model.BaseURL, ModelName: model.ModelName, HasAPIKey: model.HasAPIKey, Enabled: model.Enabled, IsDefault: model.IsDefault, SupportsReferenceImages: model.SupportsReferenceImages, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
}
func NewImageModelSummaries(models []domain.Summary) []ImageModelSummary {
	result := make([]ImageModelSummary, 0, len(models))
	for _, model := range models {
		result = append(result, NewImageModelSummary(model))
	}
	return result
}

type ImageModelListResultPayload struct {
	Models         []ImageModelSummary `json:"models"`
	DefaultModelID string              `json:"default_model_id"`
}
type ImageModelConfigAddPayload struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Protocol  string `json:"protocol"`
	AuthType  string `json:"auth_type,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	ModelName string `json:"model_name"`
	Enabled   *bool  `json:"enabled,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}
type ImageModelConfigUpdatePayload struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Protocol  string `json:"protocol"`
	AuthType  string `json:"auth_type,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	ModelName string `json:"model_name"`
}
type ImageModelConfigTestPayload struct {
	ID        string `json:"id,omitempty"`
	Protocol  string `json:"protocol"`
	AuthType  string `json:"auth_type,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	ModelName string `json:"model_name"`
}
type ImageModelConfigDeletePayload struct {
	ID string `json:"id"`
}
type ImageModelConfigEnabledSetPayload struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}
type ImageModelConfigDefaultSetPayload struct {
	ID string `json:"id"`
}
type ImageModelConfigAddResultPayload struct {
	Model          ImageModelSummary   `json:"model"`
	Models         []ImageModelSummary `json:"models"`
	DefaultModelID string              `json:"default_model_id"`
}
type ImageModelConfigTestResultPayload struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
	MIMEType  string `json:"mime_type,omitempty"`
}
type ImageModelConfigMutationResultPayload struct {
	Action         string              `json:"action"`
	Model          *ImageModelSummary  `json:"model,omitempty"`
	Models         []ImageModelSummary `json:"models"`
	DefaultModelID string              `json:"default_model_id"`
	Message        string              `json:"message"`
}

func ValidateImageModelConfigMessage(message Message) error {
	if message.RequestID == "" || len(message.RequestID) > 256 || len(message.Payload) > 64*1024 {
		return errors.New("invalid image model config request")
	}
	switch message.Type {
	case TypeImageModelList:
		if len(strings.TrimSpace(string(message.Payload))) > 0 && strings.TrimSpace(string(message.Payload)) != "{}" {
			return errors.New("invalid image model list request")
		}
	case TypeImageModelConfigAdd:
		payload, err := DecodePayload[ImageModelConfigAddPayload](message)
		if err != nil || strings.TrimSpace(payload.ID) == "" || strings.TrimSpace(payload.ModelName) == "" || !validImageModelProtocol(payload.Protocol) {
			return errors.New("invalid image model add request")
		}
	case TypeImageModelConfigUpdate:
		payload, err := DecodePayload[ImageModelConfigUpdatePayload](message)
		if err != nil || strings.TrimSpace(payload.ID) == "" || strings.TrimSpace(payload.ModelName) == "" || !validImageModelProtocol(payload.Protocol) {
			return errors.New("invalid image model update request")
		}
	case TypeImageModelConfigTest:
		payload, err := DecodePayload[ImageModelConfigTestPayload](message)
		if err != nil || strings.TrimSpace(payload.ModelName) == "" || !validImageModelProtocol(payload.Protocol) {
			return errors.New("invalid image model test request")
		}
	case TypeImageModelConfigDelete:
		payload, err := DecodePayload[ImageModelConfigDeletePayload](message)
		if err != nil || strings.TrimSpace(payload.ID) == "" {
			return errors.New("invalid image model delete request")
		}
	case TypeImageModelConfigEnabledSet:
		payload, err := DecodePayload[ImageModelConfigEnabledSetPayload](message)
		if err != nil || strings.TrimSpace(payload.ID) == "" {
			return errors.New("invalid image model enabled request")
		}
	case TypeImageModelConfigDefaultSet:
		payload, err := DecodePayload[ImageModelConfigDefaultSetPayload](message)
		if err != nil || strings.TrimSpace(payload.ID) == "" {
			return errors.New("invalid image model default request")
		}
	default:
		return errors.New("invalid image model config request type")
	}
	return nil
}
func validImageModelProtocol(value string) bool {
	return value == string(domain.ProtocolOpenAIImages) || value == string(domain.ProtocolGeminiNativeImage) || value == string(domain.ProtocolGoogleImagen)
}
