package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	intentport "myai/core/port/intent"
)

func TestJevClientSendsChoiceToConfiguredBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/v1/systemone" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("request path=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var payload struct {
			State     map[string]any `json:"state"`
			Questions map[string]any `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.State["latest_request"] != "修复代码" || payload.Questions["intent"] == nil {
			t.Errorf("request payload=%#v error=%v", payload, err)
		}
		_, _ = w.Write([]byte(`{"answers":{"intent":{"type":"choice","choice":"implementation","confidence":1}}}`))
	}))
	defer server.Close()
	client := Client{}
	body, err := client.BuildRequest("修复代码", []string{"这个项目有问题"}, "jev-latest")
	if err != nil {
		t.Fatal(err)
	}
	response, status, err := client.Send(context.Background(), intentport.Config{BaseURL: server.URL + "/proxy/v1", APIKey: "secret"}, body)
	if err != nil || status != 200 || len(response) == 0 {
		t.Fatalf("response status=%d body=%s error=%v", status, response, err)
	}
}

func TestJevClientDoesNotFollowRedirectWithSecret(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	client := Client{}
	_, status, err := client.Send(context.Background(), intentport.Config{BaseURL: server.URL, APIKey: "secret"}, []byte(`{}`))
	if err == nil || status != http.StatusFound || redirected {
		t.Fatalf("redirect result: status=%d redirected=%v error=%v", status, redirected, err)
	}
}

func TestJevClientRejectsPlaintextRemoteURL(t *testing.T) {
	client := Client{}
	if _, _, err := client.Send(context.Background(), intentport.Config{BaseURL: "http://example.com", APIKey: "secret"}, []byte(`{}`)); err == nil {
		t.Fatal("remote plaintext URL accepted")
	}
}
