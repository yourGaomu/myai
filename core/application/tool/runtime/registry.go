package runtime

import (
	"context"
	toolport "myai/core/application/tool/port"
)

type registryContextKey struct{}

func WithRegistry(ctx context.Context, registry toolport.Registry) context.Context {
	return context.WithValue(ctx, registryContextKey{}, registry)
}

func RegistryFrom(ctx context.Context) toolport.Registry {
	registry, _ := ctx.Value(registryContextKey{}).(toolport.Registry)
	return registry
}
