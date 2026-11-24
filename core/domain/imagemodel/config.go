package imagemodel

import "time"

type Protocol string

const (
	ProtocolOpenAIImages      Protocol = "openai-images"
	ProtocolGeminiNativeImage Protocol = "gemini-native-image"
	ProtocolGoogleImagen      Protocol = "google-imagen"
)

type AuthType string

const (
	AuthTypeBearer       AuthType = "bearer"
	AuthTypeGoogleAPIKey AuthType = "x-goog-api-key"
	AuthTypeGoogleADC    AuthType = "google-adc"
	AuthTypeNone         AuthType = "none"
)

func DefaultAuthType(protocol Protocol) AuthType {
	switch protocol {
	case ProtocolGeminiNativeImage:
		return AuthTypeGoogleAPIKey
	case ProtocolGoogleImagen:
		return AuthTypeGoogleADC
	default:
		return AuthTypeBearer
	}
}

type Config struct {
	ID        string
	Name      string
	Provider  string
	Protocol  Protocol
	AuthType  AuthType
	BaseURL   string
	APIKey    string
	ModelName string
	Enabled   bool
	IsDefault bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Summary struct {
	ID                      string
	Name                    string
	Provider                string
	Protocol                Protocol
	AuthType                AuthType
	BaseURL                 string
	ModelName               string
	HasAPIKey               bool
	Enabled                 bool
	IsDefault               bool
	SupportsReferenceImages bool
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func ToSummary(config Config) Summary {
	return Summary{ID: config.ID, Name: config.Name, Provider: config.Provider, Protocol: config.Protocol, AuthType: config.AuthType,
		BaseURL: config.BaseURL, ModelName: config.ModelName, HasAPIKey: config.APIKey != "", Enabled: config.Enabled,
		IsDefault: config.IsDefault, SupportsReferenceImages: config.Protocol != ProtocolGoogleImagen,
		CreatedAt: config.CreatedAt, UpdatedAt: config.UpdatedAt}
}
