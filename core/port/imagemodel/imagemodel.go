package imagemodel

import (
	"context"
	domain "myai/core/domain/imagemodel"
)

type Generator interface {
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
	SupportsReferenceImages() bool
}
type Registry interface {
	Get(string) (domain.Config, Generator, bool)
	List() []domain.Config
	Default() (domain.Config, Generator, bool)
}
type MutableRegistry interface {
	Registry
	Replace([]domain.Config, Factory) error
}
type Factory interface {
	Build(domain.Config) (Generator, error)
}
type Repository interface {
	Save(context.Context, domain.Config) error
	Get(context.Context, string) (domain.Config, error)
	List(context.Context) ([]domain.Config, error)
	Delete(context.Context, string) error
}
