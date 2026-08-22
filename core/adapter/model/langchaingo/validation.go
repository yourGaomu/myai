package langchaingo

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

func validateOpenAIConfig(config modelport.CreationConfig) error {
	if err := validateAuthType(config.AuthType, true); err != nil {
		return err
	}
	if config.BaseURL == "" {
		return errors.New("base url is empty")
	}
	if err := validateHTTPURL(config.BaseURL); err != nil {
		return err
	}
	if strings.HasSuffix(strings.ToLower(strings.TrimRight(config.BaseURL, "/")), "/chat/completions") {
		return errors.New("base url must be the API root and cannot include /chat/completions")
	}
	if config.AuthType == domainmodel.AuthTypeBearer && strings.TrimSpace(config.APIKey) == "" {
		return errors.New("api key is empty")
	}
	return nil
}

func validateAnthropicConfig(config modelport.CreationConfig) error {
	if err := validateCloudAuth(config); err != nil {
		return err
	}
	if config.BaseURL != "" {
		return validateHTTPURL(config.BaseURL)
	}
	return nil
}

func validateGoogleConfig(config modelport.CreationConfig) error {
	if err := validateCloudAuth(config); err != nil {
		return err
	}
	if config.BaseURL != "" {
		return errors.New("base url is not supported by google-generative-ai adapter")
	}
	return nil
}

func validateMistralConfig(config modelport.CreationConfig) error {
	if err := validateCloudAuth(config); err != nil {
		return err
	}
	if config.BaseURL != "" {
		return validateHTTPURL(config.BaseURL)
	}
	return nil
}

func validateOllamaConfig(config modelport.CreationConfig) error {
	if domainmodel.NormalizeAuthType(config.AuthType) != domainmodel.AuthTypeNone {
		return errors.New("ollama-chat protocol requires no authentication")
	}
	if config.BaseURL == "" {
		return errors.New("base url is empty")
	}
	if err := validateHTTPURL(config.BaseURL); err != nil {
		return err
	}
	path := strings.ToLower(strings.TrimRight(config.BaseURL, "/"))
	if strings.HasSuffix(path, "/v1") || strings.Contains(path, "/chat/completions") {
		return errors.New("ollama base url must be the server root, for example http://127.0.0.1:11434")
	}
	return nil
}

func validateCloudAuth(config modelport.CreationConfig) error {
	if domainmodel.NormalizeAuthType(config.AuthType) != domainmodel.AuthTypeBearer {
		return fmt.Errorf("%s protocol requires bearer authentication", config.Protocol)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return errors.New("api key is empty")
	}
	return nil
}

func validateAuthType(authType domainmodel.AuthType, allowNone bool) error {
	authType = domainmodel.NormalizeAuthType(authType)
	if authType == domainmodel.AuthTypeBearer {
		return nil
	}
	if allowNone && authType == domainmodel.AuthTypeNone {
		return nil
	}
	return fmt.Errorf("unsupported auth type: %s", authType)
}

func validateHTTPURL(rawURL string) error {
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return errors.New("base url must be an absolute http or https URL")
	}
	if parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return errors.New("base url cannot contain credentials, query, or fragment")
	}
	return nil
}
