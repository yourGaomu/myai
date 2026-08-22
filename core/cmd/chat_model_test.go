package cmd

import (
	"testing"

	domainmodel "myai/core/domain/model"
)

func TestModelProtocolDefaults(t *testing.T) {
	ollama, err := modelProtocolDefaults(domainmodel.ProtocolOllamaChat)
	if err != nil {
		t.Fatal(err)
	}
	if ollama.authType != domainmodel.AuthTypeNone || ollama.baseURL != "http://127.0.0.1:11434" || !ollama.baseURLRequired {
		t.Fatalf("unexpected Ollama defaults: %#v", ollama)
	}

	gemini, err := modelProtocolDefaults("gemini")
	if err != nil {
		t.Fatal(err)
	}
	if gemini.provider != "google" || gemini.authType != domainmodel.AuthTypeBearer || gemini.baseURL != "" || gemini.baseURLRequired {
		t.Fatalf("unexpected Gemini defaults: %#v", gemini)
	}
}
