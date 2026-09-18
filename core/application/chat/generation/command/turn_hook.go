package command

import (
	"context"

	"myai/core/session"
)

// AfterToolRound is invoked after a tool batch is recorded and before the
// next model turn. It must not cancel in-flight tools; those have already
// completed for this round.
type AfterToolRound func(ctx context.Context, current *session.Session) error

type afterToolRoundKey struct{}

func WithAfterToolRound(ctx context.Context, hook AfterToolRound) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if hook == nil {
		return ctx
	}
	return context.WithValue(ctx, afterToolRoundKey{}, hook)
}

func AfterToolRoundFrom(ctx context.Context) AfterToolRound {
	if ctx == nil {
		return nil
	}
	hook, _ := ctx.Value(afterToolRoundKey{}).(AfterToolRound)
	return hook
}
