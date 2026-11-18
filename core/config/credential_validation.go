package config

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateCredentials rejects example credentials before connecting to services.
// Errors identify the setting without including its value.
func (p Properties) ValidateCredentials() error {
	type credential struct {
		name  string
		value string
	}
	values := []credential{{"myai.api_key", p.Model.APIKey}}
	if strings.TrimSpace(p.Redis.Address) != "" {
		values = append(values, credential{"redis.password", p.Redis.Password})
	}
	if strings.TrimSpace(p.RAG.MinIO.Endpoint) != "" {
		values = append(values,
			credential{"rag.minio.access_key", p.RAG.MinIO.AccessKey},
			credential{"rag.minio.secret_key", p.RAG.MinIO.SecretKey},
			credential{"rag.minio.session_token", p.RAG.MinIO.SessionToken},
		)
	}
	if p.RAG.Milvus.Enabled {
		values = append(values,
			credential{"rag.milvus.password", p.RAG.Milvus.Password},
			credential{"rag.milvus.api_key", p.RAG.Milvus.APIKey},
		)
	}
	if p.Sandbox.Provider == "opensandbox" {
		values = append(values, credential{"sandbox.opensandbox.api_key", p.Sandbox.OpenSandbox.APIKey})
	}
	for index, model := range p.RAG.Embedding.Models {
		if model.Enabled != nil && !*model.Enabled {
			continue
		}
		values = append(values, credential{fmt.Sprintf("rag.embedding.models[%d].api_key", index), model.APIKey})
	}
	for _, value := range values {
		if isExampleCredential(value.value) {
			return fmt.Errorf("%s contains an example credential; provide a deployment credential", value.name)
		}
	}
	if strings.TrimSpace(p.RAG.MinIO.Endpoint) != "" {
		if strings.TrimSpace(p.RAG.MinIO.AccessKey) == "" || strings.TrimSpace(p.RAG.MinIO.SecretKey) == "" {
			return errors.New("rag.minio requires access_key and secret_key when enabled")
		}
	}
	if p.Sandbox.Provider == "opensandbox" && strings.TrimSpace(p.Sandbox.OpenSandbox.APIKey) == "" {
		return errors.New("sandbox.opensandbox.api_key is required when using opensandbox")
	}
	if p.Model.ContextWindowTokens < 0 || p.Model.MaxOutputLimit < 0 {
		return errors.New("model capacity limits must be non-negative")
	}
	if p.Model.ContextWindowTokens > 0 && p.Model.MaxOutputLimit > p.Model.ContextWindowTokens {
		return errors.New("myai.max_output_limit must not exceed myai.context_window_tokens")
	}
	return nil
}

// ValidateCredential rejects a missing or example value without echoing it.
func ValidateCredential(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if isExampleCredential(value) {
		return fmt.Errorf("%s contains an example credential; provide a deployment credential", name)
	}
	return nil
}

func isExampleCredential(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "replace-with-") || strings.HasPrefix(value, "your-") || strings.HasPrefix(value, "<") || strings.HasPrefix(value, "${") {
		return true
	}
	switch value {
	case "changeme", "change-me", "change_me", "minioadmin", "myaiadmin", "myaiadmin123456":
		return true
	default:
		return false
	}
}
