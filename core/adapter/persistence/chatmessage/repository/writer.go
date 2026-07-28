package repository

import (
	"context"
	"errors"
	"time"

	chatmessagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	chatmessageport "myai/core/adapter/persistence/chatmessage/port"
	generationcommand "myai/core/application/chat/generation/command"
	sessioncommand "myai/core/application/session/command"
	modelport "myai/core/port/model"
	portrepository "myai/core/port/repository"
	"myai/core/session"
)

type Writer struct {
	Messages    chatmessageport.MessageSaver
	Sessions    chatmessageport.SessionPersistence
	Transcripts portrepository.TranscriptRepository
	IDs         chatmessageport.IDGenerator
	Now         func() time.Time
}

func (w Writer) ReplaceSessionMessages(ctx context.Context, current *session.Session) error {
	snapshot := session.Clone(current)
	if snapshot == nil {
		return errors.New("session is nil")
	}
	if w.Transcripts == nil {
		return errors.New("transcript repository is not configured")
	}
	if w.Sessions == nil {
		return errors.New("session persistence is not configured")
	}
	mapper := chatmessagemapper.Mapper{IDs: w.IDs, Now: w.Now}
	record, err := w.Sessions.PrepareRecord(ctx, mapper.Session(snapshot, ""))
	if err != nil {
		return err
	}
	return w.Transcripts.ReplaceSessionTranscript(ctx, portrepository.TranscriptSnapshot{
		Session:  record,
		Messages: mapper.MemoryMessages(snapshot),
	})
}

func (w Writer) SaveUserMessage(ctx context.Context, command generationcommand.PersistUserMessage) error {
	var errs []error
	createdAt := command.CreatedAt
	if createdAt.IsZero() {
		createdAt = w.now()
	}
	if w.Sessions != nil {
		var err error
		if snapshot := session.Clone(command.SessionSnapshot); snapshot != nil {
			record := (chatmessagemapper.Mapper{IDs: w.IDs}).Session(snapshot, command.Title)
			record.CreatedAt = createdAt
			err = w.Sessions.SaveRecord(ctx, record)
		} else {
			err = w.Sessions.Save(ctx, sessioncommand.SaveSession{
				SessionID: command.SessionID,
				Model:     command.Model,
				Title:     command.Title,
			})
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	if w.Messages != nil {
		for _, record := range (chatmessagemapper.Mapper{IDs: w.IDs}).UserTurn(command, createdAt) {
			if err := w.Messages.SaveMessage(ctx, record); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (w Writer) SaveAssistantMessage(ctx context.Context, current *session.Session, result modelport.ChatResult, createdAt time.Time) error {
	if current == nil {
		return errors.New("session is nil")
	}
	if createdAt.IsZero() {
		createdAt = w.now()
	}

	var errs []error
	mapper := chatmessagemapper.Mapper{IDs: w.IDs}
	if w.Messages != nil {
		if err := w.Messages.SaveMessage(ctx, mapper.AssistantMessage(current.ID, result, createdAt)); err != nil {
			errs = append(errs, err)
		}
	}
	if w.Sessions != nil {
		if err := w.Sessions.SaveRecord(ctx, mapper.Session(current, "")); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (w Writer) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}
