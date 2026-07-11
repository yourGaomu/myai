package service

import (
	"context"
	"errors"
	"strings"

	sessioncommand "myai/core/application/session/command"
	lifecycleapi "myai/core/application/session/lifecycle/api"
	lifecyclecommand "myai/core/application/session/lifecycle/command"
	lifecycleport "myai/core/application/session/lifecycle/port"
	lifecycleresult "myai/core/application/session/lifecycle/result"
)

type UseCase struct {
	// UseCase 在核心生命周期操作之外，统一处理持久化、当前会话缓存和事件发布。
	Lifecycle    lifecycleport.LifecycleService
	Persistence  lifecycleport.Persistence
	Current      lifecycleport.CurrentSession
	SessionQuery lifecycleport.SessionQuery
	Events       lifecycleport.EventPublisher
}

var _ lifecycleapi.UseCase = UseCase{}

func (u UseCase) Create(ctx context.Context, command lifecyclecommand.CreateSession) (lifecycleresult.Lifecycle, error) {
	current, err := u.Lifecycle.NewSession(ctx)
	if err != nil {
		return lifecycleresult.Lifecycle{}, err
	}
	if current == nil {
		return lifecycleresult.Lifecycle{}, errors.New("created session is nil")
	}

	title := strings.TrimSpace(command.Title)
	if title == "" {
		title = "New chat"
	}
	if err := u.save(ctx, current.ID, current.Model, title); err != nil {
		return lifecycleresult.Lifecycle{}, err
	}
	if u.Current != nil {
		if err := u.Current.Save(ctx, current.ID); err != nil {
			return lifecycleresult.Lifecycle{}, err
		}
	}
	u.publish(ctx, current.ID, lifecycleresult.ActionNew)
	return lifecycleresult.Lifecycle{SessionID: current.ID, Current: current, Action: lifecycleresult.ActionNew}, nil
}

func (u UseCase) Load(ctx context.Context, command lifecyclecommand.LoadSession) (lifecycleresult.Lifecycle, error) {
	current, err := u.Lifecycle.LoadSession(ctx, command.SessionID)
	if err != nil {
		return lifecycleresult.Lifecycle{}, err
	}
	if current == nil {
		return lifecycleresult.Lifecycle{}, errors.New("loaded session is nil")
	}
	if u.Current != nil {
		if err := u.Current.Save(ctx, current.ID); err != nil {
			return lifecycleresult.Lifecycle{}, err
		}
	}
	u.publish(ctx, current.ID, lifecycleresult.ActionLoad)
	return lifecycleresult.Lifecycle{SessionID: current.ID, Current: current, Action: lifecycleresult.ActionLoad}, nil
}

func (u UseCase) Delete(ctx context.Context, command lifecyclecommand.DeleteSession) (lifecycleresult.Lifecycle, error) {
	result, err := u.Lifecycle.DeleteSession(ctx, command)
	if err != nil {
		return lifecycleresult.Lifecycle{}, err
	}

	u.publish(ctx, result.SessionID, lifecycleresult.ActionDelete)
	deleted := lifecycleresult.Lifecycle{SessionID: result.SessionID, Action: lifecycleresult.ActionDelete}
	if !result.DeletedCurrent {
		return deleted, nil
	}

	// 删除当前会话后自动切到可用会话；没有可用记录时创建新会话，保证入口始终可聊天。
	if u.SessionQuery != nil {
		records, err := u.SessionQuery.ListSessions(ctx, false)
		if err != nil {
			return lifecycleresult.Lifecycle{}, err
		}
		for _, record := range records {
			if record.ID == "" || record.ID == result.SessionID {
				continue
			}
			loaded, err := u.Load(ctx, lifecyclecommand.LoadSession{SessionID: record.ID})
			if err != nil {
				return lifecycleresult.Lifecycle{}, err
			}
			deleted.Current = loaded.Current
			return deleted, nil
		}
	}

	created, err := u.Create(ctx, lifecyclecommand.CreateSession{Title: "New chat"})
	if err != nil {
		return lifecycleresult.Lifecycle{}, err
	}
	deleted.Current = created.Current
	return deleted, nil
}

func (u UseCase) Restore(ctx context.Context, command lifecyclecommand.RestoreSession) (lifecycleresult.Lifecycle, error) {
	sessionID, err := u.Lifecycle.RestoreSession(ctx, command)
	if err != nil {
		return lifecycleresult.Lifecycle{}, err
	}
	u.publish(ctx, sessionID, lifecycleresult.ActionRestore)
	return lifecycleresult.Lifecycle{SessionID: sessionID, Action: lifecycleresult.ActionRestore}, nil
}

func (u UseCase) Clear(ctx context.Context, command lifecyclecommand.ClearSession) (lifecycleresult.Lifecycle, error) {
	current, err := u.Lifecycle.ClearCurrent(ctx)
	if err != nil {
		return lifecycleresult.Lifecycle{}, err
	}
	if current == nil {
		return lifecycleresult.Lifecycle{}, errors.New("cleared session is nil")
	}

	title := strings.TrimSpace(command.Title)
	if title == "" {
		title = "New chat"
	}
	if err := u.save(ctx, current.ID, current.Model, title); err != nil {
		return lifecycleresult.Lifecycle{}, err
	}
	u.publish(ctx, current.ID, lifecycleresult.ActionClear)
	return lifecycleresult.Lifecycle{SessionID: current.ID, Current: current, Action: lifecycleresult.ActionClear}, nil
}

func (u UseCase) save(ctx context.Context, sessionID string, model string, title string) error {
	if u.Persistence == nil {
		return nil
	}
	return u.Persistence.Save(ctx, sessioncommand.SaveSession{
		SessionID: sessionID,
		Model:     model,
		Title:     title,
	})
}

func (u UseCase) publish(ctx context.Context, sessionID string, action lifecycleresult.Action) {
	if u.Events != nil {
		u.Events.SessionChanged(ctx, sessionID, string(action))
	}
}
