package events

import (
	"context"
	"fmt"

	"myai/core/hook"
)

type Emitter interface {
	Emit(ctx context.Context, event hook.Event) error
}

type Publisher struct {
	Hooks   Emitter
	OnError func(error)
}

func (p Publisher) SessionChanged(ctx context.Context, sessionID string, reason string) {
	if p.Hooks == nil {
		return
	}
	if err := p.Hooks.Emit(ctx, hook.Event{
		Type:      hook.EventSessionChanged,
		SessionID: sessionID,
		Reason:    reason,
	}); err != nil {
		p.report(fmt.Errorf("session changed hook failed: %w", err))
	}
}

func (p Publisher) SkillReloaded(ctx context.Context, skillCount int, reason string) {
	if p.Hooks == nil {
		return
	}
	if err := p.Hooks.Emit(ctx, hook.Event{
		Type:       hook.EventSkillReloaded,
		SkillCount: skillCount,
		Reason:     reason,
	}); err != nil {
		p.report(fmt.Errorf("skill reloaded hook failed: %w", err))
	}
}

func (p Publisher) report(err error) {
	if err == nil || p.OnError == nil {
		return
	}
	p.OnError(err)
}
