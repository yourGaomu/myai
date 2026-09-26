package agent

import (
	"context"
	"errors"

	"github.com/gorilla/websocket"

	intentport "myai/core/port/intent"
	"myai/core/remote/protocol"
)

type intentFacade interface {
	IntentConfig() (intentport.ConfigView, error)
	SaveIntentConfig(context.Context, intentport.Config, bool) (intentport.ConfigView, error)
	TestIntentConfig(context.Context, intentport.Config) (int64, error)
	ListIntentTraces(context.Context, string, int) ([]intentport.Trace, error)
	GetIntentTrace(context.Context, string) (intentport.Trace, error)
	ClearIntentTraces(context.Context, string) error
}

func (a *Agent) intentService() (intentFacade, error) {
	service, ok := a.chatService.(intentFacade)
	if !ok {
		return nil, errors.New("intent settings are unavailable")
	}
	return service, nil
}

func intentConfigFromPayload(payload protocol.IntentConfigPayload) intentport.Config {
	return intentport.Config{
		Strategy: intentport.Strategy(payload.Strategy), BaseURL: payload.BaseURL,
		APIKey: payload.APIKey, Model: payload.Model,
		PlanConfidence: payload.PlanConfidence, ExecuteConfidence: payload.ExecuteConfidence,
	}
}

func intentConfigResult(config intentport.ConfigView) protocol.IntentConfigResultPayload {
	return protocol.IntentConfigResultPayload{
		Strategy: string(config.Strategy), BaseURL: config.BaseURL, HasAPIKey: config.HasAPIKey,
		Model: config.Model, PlanConfidence: config.PlanConfidence, ExecuteConfidence: config.ExecuteConfidence,
	}
}

func intentTraceResult(trace intentport.Trace) protocol.IntentTracePayload {
	return protocol.IntentTracePayload{
		TraceID: trace.ID, SessionID: trace.SessionID, RequestID: trace.RequestID, CreatedAt: trace.CreatedAt,
		DurationMS: trace.DurationMS, BaseURL: trace.BaseURL, RequestedModel: trace.RequestedModel,
		ResponseModel: trace.ResponseModel, RequestBody: trace.RequestBody, ResponseBody: trace.ResponseBody,
		ResponseTruncated: trace.ResponseTruncated, HTTPStatus: trace.HTTPStatus,
		Status: trace.Status, ErrorCode: trace.ErrorCode, Choice: trace.Choice,
		Confidence: trace.Confidence, ShouldPlan: trace.ShouldPlan,
		ShouldExecute: trace.ShouldExecute, Route: trace.Route,
	}
}

func (a *Agent) handleIntentConfigQuery(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	service, err := a.intentService()
	if err != nil {
		return err
	}
	config, err := service.IntentConfig()
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeIntentConfigQueryResult, message.RequestID, message.SessionID, intentConfigResult(config))
}

func (a *Agent) handleIntentConfigSet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.IntentConfigPayload](message)
	if err != nil {
		return err
	}
	service, err := a.intentService()
	if err != nil {
		return err
	}
	config, err := service.SaveIntentConfig(ctx, intentConfigFromPayload(payload), payload.ClearAPIKey)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeIntentConfigSetResult, message.RequestID, message.SessionID, intentConfigResult(config))
}

func (a *Agent) handleIntentConfigTest(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.IntentConfigPayload](message)
	if err != nil {
		return err
	}
	service, err := a.intentService()
	if err != nil {
		return err
	}
	latency, err := service.TestIntentConfig(ctx, intentConfigFromPayload(payload))
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeIntentConfigTestResult, message.RequestID, message.SessionID, protocol.IntentConfigTestResultPayload{Success: true, LatencyMS: latency})
}

func (a *Agent) handleIntentTraceList(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.IntentTraceListPayload](message)
	if err != nil {
		return err
	}
	service, err := a.intentService()
	if err != nil {
		return err
	}
	traces, err := service.ListIntentTraces(ctx, payload.SessionID, payload.Limit)
	if err != nil {
		return err
	}
	items := make([]protocol.IntentTracePayload, 0, len(traces))
	for _, trace := range traces {
		items = append(items, intentTraceResult(trace))
	}
	return a.writeRemoteMessage(conn, protocol.TypeIntentTraceListResult, message.RequestID, message.SessionID, protocol.IntentTraceListResultPayload{Traces: items})
}

func (a *Agent) handleIntentTraceGet(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.IntentTraceGetPayload](message)
	if err != nil {
		return err
	}
	service, err := a.intentService()
	if err != nil {
		return err
	}
	trace, err := service.GetIntentTrace(ctx, payload.TraceID)
	if err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeIntentTraceGetResult, message.RequestID, message.SessionID, protocol.IntentTraceGetResultPayload{Trace: intentTraceResult(trace)})
}

func (a *Agent) handleIntentTraceClear(ctx context.Context, conn *websocket.Conn, message protocol.Message) error {
	payload, err := protocol.DecodePayload[protocol.IntentTraceClearPayload](message)
	if err != nil {
		return err
	}
	service, err := a.intentService()
	if err != nil {
		return err
	}
	if err := service.ClearIntentTraces(ctx, payload.SessionID); err != nil {
		return err
	}
	return a.writeRemoteMessage(conn, protocol.TypeIntentTraceClearResult, message.RequestID, message.SessionID, protocol.IntentTraceClearResultPayload{SessionID: payload.SessionID})
}
