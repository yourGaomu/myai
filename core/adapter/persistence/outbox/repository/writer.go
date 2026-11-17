package repository

import (
	"context"
	"errors"

	outbox "myai/core/port/outbox"
	repository "myai/core/port/repository"
)

type MessageSaver interface {
	SaveMessage(context.Context, repository.MessageRecord) error
}
type AssetSaver interface {
	SaveAsset(context.Context, repository.AssetRecord) error
}

// Writer applies only immutable appends and narrow message metadata updates.
// Partial success is safe to repeat with precisely the same mapped record IDs.
type SessionReader interface {
	GetSession(context.Context, string) (repository.SessionRecord, error)
}

type Writer struct {
	Messages MessageSaver
	Assets   AssetSaver
	Metadata repository.SessionMessageMetadataStore
	Sessions SessionReader
}

func (w Writer) Apply(ctx context.Context, batch outbox.Batch) error {
	if err := batch.Validate(); err != nil {
		return err
	}
	if len(batch.Messages) > 0 && w.Messages == nil {
		return errors.New("outbox message backend is unavailable")
	}
	if len(batch.Assets) > 0 && w.Assets == nil {
		return errors.New("outbox asset backend is unavailable")
	}
	if batch.Metadata != nil && w.Metadata == nil {
		return errors.New("outbox metadata backend is unavailable")
	}
	if w.Sessions == nil {
		return errors.New("outbox session backend is unavailable")
	}
	current, err := w.Sessions.GetSession(ctx, batch.SessionID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return outbox.SafeError("read outbox session", err)
	}
	if err == nil && current.Deleted {
		return outbox.ErrBlocked
	}
	if errors.Is(err, repository.ErrNotFound) && (batch.Metadata == nil || batch.Metadata.RequiresExisting) {
		return outbox.SafeError("read existing outbox session", repository.ErrNotFound)
	}
	for _, record := range batch.Messages {
		if err := w.Messages.SaveMessage(ctx, record); err != nil {
			return outbox.SafeError("save outbox message", err)
		}
	}
	for _, record := range batch.Assets {
		if err := w.Assets.SaveAsset(ctx, record); err != nil {
			return outbox.SafeError("save outbox asset", err)
		}
	}
	if m := batch.Metadata; m != nil {
		if err := w.Metadata.TouchSessionMessages(ctx, m.SessionRecord(batch.SessionID), m.Sequence, m.IncludeUsage); err != nil {
			return outbox.SafeError("touch outbox message metadata", err)
		}
	}
	return nil
}
