package service

import (
	"context"
	"errors"

	intentport "myai/core/port/intent"
)

func (s *ChatService) IntentConfig() (intentport.ConfigView, error) {
	if s.dependencies.IntentController == nil {
		return intentport.ConfigView{}, errors.New("intent controller is unavailable")
	}
	return s.dependencies.IntentController.Config(), nil
}

func (s *ChatService) SaveIntentConfig(ctx context.Context, config intentport.Config, clearAPIKey bool) (intentport.ConfigView, error) {
	if s.dependencies.IntentController == nil {
		return intentport.ConfigView{}, errors.New("intent controller is unavailable")
	}
	return s.dependencies.IntentController.SaveConfig(ctx, config, clearAPIKey)
}

func (s *ChatService) TestIntentConfig(ctx context.Context, config intentport.Config) (int64, error) {
	if s.dependencies.IntentController == nil {
		return 0, errors.New("intent controller is unavailable")
	}
	return s.dependencies.IntentController.TestJev(ctx, config)
}

func (s *ChatService) ListIntentTraces(ctx context.Context, sessionID string, limit int) ([]intentport.Trace, error) {
	if s.dependencies.IntentController == nil {
		return nil, errors.New("intent controller is unavailable")
	}
	return s.dependencies.IntentController.ListTraces(ctx, sessionID, limit)
}

func (s *ChatService) GetIntentTrace(ctx context.Context, traceID string) (intentport.Trace, error) {
	if s.dependencies.IntentController == nil {
		return intentport.Trace{}, errors.New("intent controller is unavailable")
	}
	return s.dependencies.IntentController.GetTrace(ctx, traceID)
}

func (s *ChatService) ClearIntentTraces(ctx context.Context, sessionID string) error {
	if s.dependencies.IntentController == nil {
		return errors.New("intent controller is unavailable")
	}
	return s.dependencies.IntentController.ClearTraces(ctx, sessionID)
}
