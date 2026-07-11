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
	"myai/core/session"
)

type Writer struct {
	Messages chatmessageport.MessageSaver
	Sessions chatmessageport.SessionPersistence
	IDs      chatmessageport.IDGenerator
	Now      func() time.Time
}

func (w Writer) SaveUserMessage(ctx context.Context, command generationcommand.PersistUserMessage) error {
	var errs []error
	if w.Sessions != nil {
		if err := w.Sessions.Save(ctx, sessioncommand.SaveSession{
			SessionID: command.SessionID,
			Model:     command.Model,
			Title:     command.Title,
		}); err != nil {
			errs = append(errs, err)
		}
	}
	if w.Messages != nil {
		record := (chatmessagemapper.Mapper{IDs: w.IDs}).UserMessage(command, w.now())
		if err := w.Messages.SaveMessage(ctx, record); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (w Writer) SaveAssistantMessage(ctx context.Context, current *session.Session, result modelport.ChatResult) error {
	if current == nil {
		return errors.New("session is nil")
	}

	var errs []error
	mapper := chatmessagemapper.Mapper{IDs: w.IDs}
	if w.Messages != nil {
		if err := w.Messages.SaveMessage(ctx, mapper.AssistantMessage(current.ID, result, w.now())); err != nil {
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
