package intent

import (
	"context"
	"errors"
	"time"
)

type Strategy string

const (
	StrategySystem Strategy = "system"
	StrategyJev    Strategy = "jev"
	StrategyOff    Strategy = "off"
)

var ErrNotFound = errors.New("intent record not found")
var ErrResponseTooLarge = errors.New("Jev response exceeds 64 KiB")

type Config struct {
	Strategy          Strategy
	BaseURL           string
	APIKey            string
	Model             string
	PlanConfidence    float64
	ExecuteConfidence float64
}

type ConfigView struct {
	Strategy          Strategy
	BaseURL           string
	HasAPIKey         bool
	Model             string
	PlanConfidence    float64
	ExecuteConfidence float64
}

func (c Config) View() ConfigView {
	return ConfigView{c.Strategy, c.BaseURL, c.APIKey != "", c.Model, c.PlanConfidence, c.ExecuteConfidence}
}

func DefaultConfig() Config {
	return Config{Strategy: StrategySystem, BaseURL: "https://api.typesafe.ai", Model: "jev-latest", PlanConfidence: 0.55, ExecuteConfidence: 0.85}
}

type Trace struct {
	ID                string
	SessionID         string
	RequestID         string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	DurationMS        int64
	BaseURL           string
	RequestedModel    string
	ResponseModel     string
	RequestBody       string
	ResponseBody      string
	ResponseTruncated bool
	HTTPStatus        int
	Status            string
	ErrorCode         string
	Choice            string
	Confidence        float64
	ShouldPlan        bool
	ShouldExecute     bool
	Route             string
}

type requestIDContextKey struct{}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}
