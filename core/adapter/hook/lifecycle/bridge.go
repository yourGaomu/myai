package lifecycle

import (
	"context"

	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	"myai/core/hook"
)

type Bridge struct {
	Hooks *hook.Manager
}

var _ generationport.TurnLifecycleHooks = Bridge{}

func (b Bridge) Handle(ctx context.Context, command generationcommand.TurnHook) (generationcommand.TurnHookOutcome, error) {
	if b.Hooks == nil {
		return generationcommand.TurnHookOutcome{}, nil
	}
	prompt := command.Prompt
	if command.Kind == generationcommand.TurnHookStop && prompt == "" {
		prompt = command.LastAssistant
	}
	result, err := b.Hooks.RunLifecycle(ctx, hook.Event{
		Type:      hook.EventType(command.Kind),
		SessionID: command.SessionID,
		Prompt:    prompt,
		Reason:    string(command.Kind),
	})
	if err != nil {
		return generationcommand.TurnHookOutcome{}, err
	}
	return generationcommand.TurnHookOutcome{
		Denied:       result.Decision == hook.DecisionDeny,
		Continuation: result.Message,
	}, nil
}
