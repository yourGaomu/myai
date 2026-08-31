package runtime

import "context"

type runIDKey struct{}
type metadataKey struct{}

type Metadata struct {
	RunID       string
	ParentRunID string
	PlanID      string
	StepID      string
	TaskID      string
}

func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey{}, runID)
}

func RunID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	runID, _ := ctx.Value(runIDKey{}).(string)
	return runID
}

func WithMetadata(ctx context.Context, metadata Metadata) context.Context {
	return context.WithValue(ctx, metadataKey{}, metadata)
}

func MetadataFrom(ctx context.Context) Metadata {
	if ctx == nil {
		return Metadata{}
	}
	metadata, _ := ctx.Value(metadataKey{}).(Metadata)
	return metadata
}
