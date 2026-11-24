package api

import (
	"context"
	"myai/core/application/imagemodel/result"
	domain "myai/core/domain/imagemodel"
)

type ConfigService interface {
	List(context.Context) ([]domain.Summary, string, error)
	Add(context.Context, domain.Config) (result.MutationResult, error)
	Test(context.Context, domain.Config) (result.TestResult, error)
	Update(context.Context, domain.Config) (result.MutationResult, error)
	Delete(context.Context, string) (result.MutationResult, error)
	SetEnabled(context.Context, string, bool) (result.MutationResult, error)
	SetDefault(context.Context, string) (result.MutationResult, error)
}
