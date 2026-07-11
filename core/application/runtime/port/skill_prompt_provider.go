package port

import "context"

type SkillPromptProvider interface {
	PromptForInput(ctx context.Context, input string) string
}
