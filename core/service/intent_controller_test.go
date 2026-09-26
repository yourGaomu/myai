package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jevclient "myai/core/adapter/intent/jev"
	intentstore "myai/core/adapter/intent/store"
	intentport "myai/core/port/intent"
	"myai/core/session"
)

func TestIntentControllerJevRoutesAndRecordsExactBodies(t *testing.T) {
	confidence := 0.94
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("unexpected Jev request: path=%s authorization=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.13.0", "answers": map[string]any{"intent": map[string]any{
				"type": "choice", "choice": "implementation", "confidence": confidence,
				"probabilities": map[string]float64{"implementation": 0.95, "conversation": 0.03, "explanation": 0.02},
			}},
		})
	}))
	defer server.Close()
	store := intentstore.NewMemory()
	controller, err := NewIntentController(context.Background(), store, jevclient.Client{}, RuleBasedAutoPlanClassifier{})
	if err != nil {
		t.Fatal(err)
	}
	config := intentport.DefaultConfig()
	config.Strategy, config.BaseURL, config.APIKey = intentport.StrategyJev, server.URL, "test-secret"
	if _, err := controller.SaveConfig(context.Background(), config, false); err != nil {
		t.Fatal(err)
	}
	current := &session.Session{ID: "session-1", Kind: session.KindUser}
	decision, err := controller.Classify(intentport.WithRequestID(context.Background(), "request-1"), current, "实现这个功能")
	if err != nil || !decision.ShouldPlan || !decision.ShouldExecute {
		t.Fatalf("high-confidence decision = %#v, %v", decision, err)
	}
	confidence = 0.7
	decision, err = controller.Classify(context.Background(), current, "实现另一个功能")
	if err != nil || !decision.ShouldPlan || decision.ShouldExecute {
		t.Fatalf("medium-confidence decision = %#v, %v", decision, err)
	}
	confidence = 0.3
	decision, err = controller.Classify(context.Background(), current, "实现第三个功能")
	if err != nil || decision.ShouldPlan || decision.ShouldExecute {
		t.Fatalf("low-confidence decision = %#v, %v", decision, err)
	}
	traces, err := controller.ListTraces(context.Background(), current.ID, 10)
	if err != nil || len(traces) != 3 {
		t.Fatalf("traces = %#v, %v", traces, err)
	}
	for _, trace := range traces {
		if trace.RequestBody != "" || trace.ResponseBody != "" {
			t.Fatal("trace list must not include raw bodies")
		}
		detail, err := controller.GetTrace(context.Background(), trace.ID)
		if err != nil || detail.Status != "succeeded" || !strings.Contains(detail.RequestBody, "latest_request") || !strings.Contains(detail.ResponseBody, "probabilities") {
			t.Fatalf("trace detail = %#v, %v", detail, err)
		}
		if strings.Contains(detail.RequestBody+detail.ResponseBody, "test-secret") {
			t.Fatal("trace leaked API key")
		}
	}
	foundRequest := false
	for _, trace := range traces {
		if trace.RequestID == "request-1" {
			foundRequest = true
		}
	}
	if !foundRequest {
		t.Fatal("trace did not retain the client request ID")
	}
	if !controller.Config().HasAPIKey {
		t.Fatal("configuration must report that a key exists")
	}
	encoded, _ := json.Marshal(controller.Config())
	if strings.Contains(string(encoded), "test-secret") {
		t.Fatal("configuration response leaked API key")
	}
}

func TestIntentControllerFailsClosedAndRecordsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"intent":{"type":"choice","choice":"implementation"}}}`))
	}))
	defer server.Close()
	store := intentstore.NewMemory()
	controller, err := NewIntentController(context.Background(), store, jevclient.Client{}, RuleBasedAutoPlanClassifier{})
	if err != nil {
		t.Fatal(err)
	}
	config := intentport.DefaultConfig()
	config.Strategy, config.BaseURL, config.APIKey = intentport.StrategyJev, server.URL, "secret"
	if _, err := controller.SaveConfig(context.Background(), config, false); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Classify(context.Background(), &session.Session{ID: "session-1"}, "修复代码"); err == nil {
		t.Fatal("missing confidence must fail closed")
	}
	traces, err := controller.ListTraces(context.Background(), "session-1", 10)
	if err != nil || len(traces) != 1 || traces[0].Status != "invalid_response" {
		t.Fatalf("invalid response trace = %#v, %v", traces, err)
	}
	if _, err := controller.SaveConfig(context.Background(), intentport.Config{
		Strategy: intentport.StrategyOff, BaseURL: server.URL, Model: "jev-latest",
		PlanConfidence: 0.55, ExecuteConfidence: 0.85,
	}, false); err != nil {
		t.Fatal(err)
	}
	decision, err := controller.Classify(context.Background(), &session.Session{ID: "session-1"}, "修复代码")
	if err != nil || decision.ShouldPlan {
		t.Fatalf("off strategy = %#v, %v", decision, err)
	}
}

func TestIntentControllerRecordsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()
	store := intentstore.NewMemory()
	controller, err := NewIntentController(context.Background(), store, jevclient.Client{}, RuleBasedAutoPlanClassifier{})
	if err != nil {
		t.Fatal(err)
	}
	config := intentport.DefaultConfig()
	config.Strategy, config.BaseURL, config.APIKey = intentport.StrategyJev, server.URL, "secret"
	if _, err := controller.SaveConfig(context.Background(), config, false); err != nil {
		t.Fatal(err)
	}
	if decision, err := controller.Classify(context.Background(), &session.Session{ID: "session-1"}, "修改代码"); err == nil || decision.ShouldPlan {
		t.Fatalf("401 must fail closed: %#v, %v", decision, err)
	}
	traces, err := controller.ListTraces(context.Background(), "session-1", 10)
	if err != nil || len(traces) != 1 || traces[0].Status != "http_error" || traces[0].HTTPStatus != 401 {
		t.Fatalf("HTTP failure trace = %#v, %v", traces, err)
	}
}

type failingIntentTraceStore struct {
	*intentstore.Memory
	saves int
}

func (s *failingIntentTraceStore) SaveTrace(ctx context.Context, trace intentport.Trace) error {
	s.saves++
	if s.saves >= 2 {
		return context.DeadlineExceeded
	}
	return s.Memory.SaveTrace(ctx, trace)
}

func TestIntentControllerDoesNotExecuteWithoutFinalTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"intent":{"type":"choice","choice":"implementation","confidence":1}}}`))
	}))
	defer server.Close()
	store := &failingIntentTraceStore{Memory: intentstore.NewMemory()}
	controller, err := NewIntentController(context.Background(), store, jevclient.Client{}, RuleBasedAutoPlanClassifier{})
	if err != nil {
		t.Fatal(err)
	}
	config := intentport.DefaultConfig()
	config.Strategy, config.BaseURL, config.APIKey = intentport.StrategyJev, server.URL, "secret"
	if _, err := controller.SaveConfig(context.Background(), config, false); err != nil {
		t.Fatal(err)
	}
	if decision, err := controller.Classify(context.Background(), &session.Session{ID: "session-1"}, "修复代码"); err == nil || decision.ShouldExecute {
		t.Fatalf("missing final trace must fail closed: %#v, %v", decision, err)
	}
}

func TestIntentConfigCanSwitchStrategyWithoutResendingSecret(t *testing.T) {
	controller, err := NewIntentController(context.Background(), intentstore.NewMemory(), jevclient.Client{}, RuleBasedAutoPlanClassifier{})
	if err != nil {
		t.Fatal(err)
	}
	config := intentport.DefaultConfig()
	config.Strategy, config.APIKey = intentport.StrategyJev, "secret"
	if _, err := controller.SaveConfig(context.Background(), config, false); err != nil {
		t.Fatal(err)
	}
	view, err := controller.SaveConfig(context.Background(), intentport.Config{Strategy: intentport.StrategyOff}, false)
	if err != nil || view.Strategy != intentport.StrategyOff || !view.HasAPIKey || view.BaseURL != config.BaseURL {
		t.Fatalf("partial strategy update = %#v, %v", view, err)
	}
	if _, err := controller.SaveConfig(context.Background(), intentport.Config{Strategy: intentport.StrategyJev, BaseURL: "http://example.com"}, false); err == nil {
		t.Fatal("remote plaintext HTTP must be rejected")
	}
	view, err = controller.SaveConfig(context.Background(), intentport.Config{Strategy: intentport.StrategyOff}, true)
	if err != nil || view.HasAPIKey {
		t.Fatalf("clear key = %#v, %v", view, err)
	}
}
