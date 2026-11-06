package lifecycle

import (
	"context"
	"fmt"
	"log"
	"strings"

	generationcommand "myai/core/application/chat/generation/command"
	generationport "myai/core/application/chat/generation/port"
	"myai/core/hook"
)

type Bridge struct {
	Hooks   *hook.Manager
	OnError func(error)
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
		if command.Kind == generationcommand.TurnHookStop {
			// Stop observes an already generated answer. A failed hook must not
			// discard that answer or turn diagnostic output into another prompt.
			err = fmt.Errorf("stop hook failed: %w", err)
			if b.OnError != nil {
				b.OnError(err)
			} else {
				log.Printf("%v", err)
			}
			return generationcommand.TurnHookOutcome{}, nil
		}
		return generationcommand.TurnHookOutcome{}, err
	}
	outcome := generationcommand.TurnHookOutcome{Denied: result.Decision == hook.DecisionDeny}
	if command.Kind == generationcommand.TurnHookStop && outcome.Denied {
		outcome.Continuation = strings.TrimSpace(result.Message)
		if outcome.Continuation == "" {
			outcome.Continuation = "A Stop hook requested further work. Review the answer and complete any remaining work before stopping."
		}
	} else if command.Kind != generationcommand.TurnHookStop {
		outcome.Continuation = result.Message
	}
	return outcome, nil
}
