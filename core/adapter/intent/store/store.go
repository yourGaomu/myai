package store

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	intentport "myai/core/port/intent"
)

const (
	configCollection = "intent_classifier_config"
	traceCollection  = "intent_judgment_traces"
)

type Memory struct {
	mu        sync.RWMutex
	config    *intentport.Config
	traces    map[string]intentport.Trace
	lastPrune time.Time
}

func NewMemory() *Memory { return &Memory{traces: make(map[string]intentport.Trace)} }

var _ intentport.Store = (*Memory)(nil)

func (m *Memory) LoadConfig(context.Context) (intentport.Config, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config == nil {
		return intentport.Config{}, intentport.ErrNotFound
	}
	return *m.config, nil
}

func (m *Memory) SaveConfig(_ context.Context, config intentport.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = &config
	return nil
}

func (m *Memory) SaveTrace(_ context.Context, trace intentport.Trace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.lastPrune) >= time.Hour {
		now := time.Now()
		for id, existing := range m.traces {
			if !now.Before(existing.ExpiresAt) {
				delete(m.traces, id)
			}
		}
		m.lastPrune = now
	}
	m.traces[trace.ID] = trace
	return nil
}

func (m *Memory) ListTraces(_ context.Context, sessionID string, limit int) ([]intentport.Trace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]intentport.Trace, 0, len(m.traces))
	for _, trace := range m.traces {
		if time.Now().After(trace.ExpiresAt) || (sessionID != "" && trace.SessionID != sessionID) {
			continue
		}
		trace.RequestBody = ""
		trace.ResponseBody = ""
		items = append(items, trace)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (m *Memory) GetTrace(_ context.Context, id string) (intentport.Trace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	trace, ok := m.traces[id]
	if !ok || time.Now().After(trace.ExpiresAt) {
		return intentport.Trace{}, intentport.ErrNotFound
	}
	return trace, nil
}

func (m *Memory) ClearTraces(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, trace := range m.traces {
		if sessionID == "" || trace.SessionID == sessionID {
			delete(m.traces, id)
		}
	}
	return nil
}

type Mongo struct{ database *mongo.Database }

var _ intentport.Store = (*Mongo)(nil)

type configDocument struct {
	ID                string              `bson:"_id"`
	Strategy          intentport.Strategy `bson:"strategy"`
	BaseURL           string              `bson:"base_url"`
	APIKey            string              `bson:"api_key"`
	Model             string              `bson:"model"`
	PlanConfidence    float64             `bson:"plan_confidence"`
	ExecuteConfidence float64             `bson:"execute_confidence"`
}

func configDocumentFromDomain(config intentport.Config) configDocument {
	return configDocument{"default", config.Strategy, config.BaseURL, config.APIKey, config.Model, config.PlanConfidence, config.ExecuteConfidence}
}

func (document configDocument) domain() intentport.Config {
	return intentport.Config{Strategy: document.Strategy, BaseURL: document.BaseURL, APIKey: document.APIKey, Model: document.Model, PlanConfidence: document.PlanConfidence, ExecuteConfidence: document.ExecuteConfidence}
}

type traceDocument struct {
	ID                string    `bson:"_id"`
	SessionID         string    `bson:"session_id"`
	RequestID         string    `bson:"request_id"`
	CreatedAt         time.Time `bson:"created_at"`
	ExpiresAt         time.Time `bson:"expires_at"`
	DurationMS        int64     `bson:"duration_ms"`
	BaseURL           string    `bson:"base_url"`
	RequestedModel    string    `bson:"requested_model"`
	ResponseModel     string    `bson:"response_model"`
	RequestBody       string    `bson:"request_body"`
	ResponseBody      string    `bson:"response_body"`
	ResponseTruncated bool      `bson:"response_truncated"`
	HTTPStatus        int       `bson:"http_status"`
	Status            string    `bson:"status"`
	ErrorCode         string    `bson:"error_code"`
	Choice            string    `bson:"choice"`
	Confidence        float64   `bson:"confidence"`
	ShouldPlan        bool      `bson:"should_plan"`
	ShouldExecute     bool      `bson:"should_execute"`
	Route             string    `bson:"route"`
}

func traceDocumentFromDomain(trace intentport.Trace) traceDocument {
	return traceDocument{trace.ID, trace.SessionID, trace.RequestID, trace.CreatedAt, trace.ExpiresAt,
		trace.DurationMS, trace.BaseURL, trace.RequestedModel, trace.ResponseModel, trace.RequestBody,
		trace.ResponseBody, trace.ResponseTruncated, trace.HTTPStatus, trace.Status, trace.ErrorCode,
		trace.Choice, trace.Confidence, trace.ShouldPlan, trace.ShouldExecute, trace.Route}
}

func (document traceDocument) domain() intentport.Trace {
	return intentport.Trace{ID: document.ID, SessionID: document.SessionID, RequestID: document.RequestID,
		CreatedAt: document.CreatedAt, ExpiresAt: document.ExpiresAt, DurationMS: document.DurationMS,
		BaseURL: document.BaseURL, RequestedModel: document.RequestedModel, ResponseModel: document.ResponseModel,
		RequestBody: document.RequestBody, ResponseBody: document.ResponseBody, ResponseTruncated: document.ResponseTruncated,
		HTTPStatus: document.HTTPStatus, Status: document.Status, ErrorCode: document.ErrorCode,
		Choice: document.Choice, Confidence: document.Confidence, ShouldPlan: document.ShouldPlan,
		ShouldExecute: document.ShouldExecute, Route: document.Route}
}

func NewMongo(ctx context.Context, client *mongo.Client, database string) (*Mongo, error) {
	if client == nil || strings.TrimSpace(database) == "" {
		return nil, errors.New("MongoDB is not configured")
	}
	store := &Mongo{database: client.Database(database)}
	_, err := store.database.Collection(traceCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0),
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (m *Mongo) LoadConfig(ctx context.Context) (intentport.Config, error) {
	var document configDocument
	err := m.database.Collection(configCollection).FindOne(ctx, bson.M{"_id": "default"}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return intentport.Config{}, intentport.ErrNotFound
	}
	return document.domain(), err
}

func (m *Mongo) SaveConfig(ctx context.Context, config intentport.Config) error {
	_, err := m.database.Collection(configCollection).ReplaceOne(ctx, bson.M{"_id": "default"}, configDocumentFromDomain(config), options.Replace().SetUpsert(true))
	return err
}

func (m *Mongo) SaveTrace(ctx context.Context, trace intentport.Trace) error {
	_, err := m.database.Collection(traceCollection).ReplaceOne(ctx, bson.M{"_id": trace.ID}, traceDocumentFromDomain(trace), options.Replace().SetUpsert(true))
	return err
}

func (m *Mongo) ListTraces(ctx context.Context, sessionID string, limit int) ([]intentport.Trace, error) {
	filter := bson.M{"expires_at": bson.M{"$gt": time.Now()}}
	if sessionID != "" {
		filter["session_id"] = sessionID
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	cursor, err := m.database.Collection(traceCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)).SetProjection(bson.M{"request_body": 0, "response_body": 0}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var documents []traceDocument
	if err := cursor.All(ctx, &documents); err != nil {
		return nil, err
	}
	items := make([]intentport.Trace, 0, len(documents))
	for _, document := range documents {
		items = append(items, document.domain())
	}
	return items, nil
}

func (m *Mongo) GetTrace(ctx context.Context, id string) (intentport.Trace, error) {
	var document traceDocument
	err := m.database.Collection(traceCollection).FindOne(ctx, bson.M{"_id": id, "expires_at": bson.M{"$gt": time.Now()}}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return intentport.Trace{}, intentport.ErrNotFound
	}
	return document.domain(), err
}

func (m *Mongo) ClearTraces(ctx context.Context, sessionID string) error {
	filter := bson.M{}
	if sessionID != "" {
		filter["session_id"] = sessionID
	}
	_, err := m.database.Collection(traceCollection).DeleteMany(ctx, filter)
	return err
}
