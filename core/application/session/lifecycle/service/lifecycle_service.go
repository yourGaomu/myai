package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	lifecyclecommand "myai/core/application/session/lifecycle/command"
	lifecycleport "myai/core/application/session/lifecycle/port"
	lifecycleresult "myai/core/application/session/lifecycle/result"
	"myai/core/port/repository"
	"myai/core/session"
)

type LifecycleService struct {
	// LifecycleService 处理内存和仓库的原子操作；跨用例副作用由外层 UseCase 编排。
	Memory   lifecycleport.MemoryStore
	Loader   lifecycleport.SessionLoader
	Sessions lifecycleport.SessionRepository
	Messages lifecycleport.MessageRepository
	Now      func() time.Time
}

var _ lifecycleport.LifecycleService = LifecycleService{}

func (s LifecycleService) NewSession(ctx context.Context) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	if err := s.Memory.NewSession(); err != nil {
		return nil, err
	}
	return s.Memory.Current()
}

func (s LifecycleService) LoadSession(ctx context.Context, sessionID string) (*session.Session, error) {
	if s.Loader != nil {
		return s.Loader.LoadCurrent(ctx, sessionID)
	}
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	current, err := s.Memory.GetSession(strings.TrimSpace(sessionID))
	if err != nil {
		return nil, errors.New("session loader is nil")
	}
	if err := s.Memory.UseSession(current.ID); err != nil {
		return nil, err
	}
	return current, nil
}

func (s LifecycleService) DeleteSession(ctx context.Context, command lifecyclecommand.DeleteSession) (lifecycleresult.DeleteSession, error) {
	if s.Memory == nil {
		return lifecycleresult.DeleteSession{}, errors.New("session manager is nil")
	}

	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		sessionID = s.Memory.CurrentSessionId()
	}
	if sessionID == "" {
		return lifecycleresult.DeleteSession{}, errors.New("session id is empty")
	}

	// 先软删除持久化记录，再移除内存对象，避免数据库失败后会话在进程内提前消失。
	deletingCurrent := sessionID == s.Memory.CurrentSessionId()
	if s.Sessions != nil {
		if _, err := s.Sessions.GetSession(ctx, sessionID); err != nil {
			return lifecycleresult.DeleteSession{}, err
		}
		if err := s.Sessions.MarkSessionDeleted(ctx, sessionID, s.now()); err != nil {
			return lifecycleresult.DeleteSession{}, err
		}
	}
	if err := s.Memory.RemoveSession(sessionID); err != nil {
		return lifecycleresult.DeleteSession{}, err
	}
	return lifecycleresult.DeleteSession{
		SessionID:      sessionID,
		DeletedCurrent: deletingCurrent,
	}, nil
}

func (s LifecycleService) RenameSession(ctx context.Context, command lifecyclecommand.RenameSession) (lifecycleresult.RenameSession, error) {
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return lifecycleresult.RenameSession{}, errors.New("session id is empty")
	}
	title := strings.TrimSpace(command.Title)
	if title == "" {
		return lifecycleresult.RenameSession{}, errors.New("session title is empty")
	}
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > 100 {
		return lifecycleresult.RenameSession{}, errors.New("session title must be valid UTF-8 and at most 100 Unicode code points")
	}
	store, ok := s.Sessions.(repository.SessionTitleRepository)
	if !ok {
		return lifecycleresult.RenameSession{}, errors.New("session repository does not support renaming")
	}
	record, err := store.RenameSession(ctx, sessionID, title)
	if errors.Is(err, repository.ErrNotFound) {
		return lifecycleresult.RenameSession{}, fmt.Errorf("active user session %q not found: %w", sessionID, err)
	}
	if err != nil {
		return lifecycleresult.RenameSession{}, err
	}
	return lifecycleresult.RenameSession{SessionID: record.ID, Title: record.DisplayTitle(), TitleRevision: record.TitleRevision}, nil
}

func (s LifecycleService) RestoreSession(ctx context.Context, command lifecyclecommand.RestoreSession) (string, error) {
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return "", errors.New("session id is empty")
	}
	if s.Sessions == nil {
		return "", errors.New("store is nil")
	}
	if err := s.Sessions.MarkSessionRestored(ctx, sessionID, s.now()); err != nil {
		return "", err
	}
	return sessionID, nil
}

// RestoreDeletedSession checks the store before changing it. A session left
// blocked by a partial clear remains blocked across restart and must be cleared
// again explicitly rather than resuming an uncertain transcript.
func (s LifecycleService) RestoreDeletedSession(ctx context.Context, command lifecyclecommand.RestoreSession) (string, error) {
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return "", errors.New("session id is empty")
	}
	if s.Sessions == nil {
		return "", errors.New("store is nil")
	}
	getSession := s.Sessions.GetSession
	if store, ok := s.Sessions.(interface {
		GetSessionIncludingDeleted(context.Context, string) (repository.SessionRecord, error)
	}); ok {
		getSession = store.GetSessionIncludingDeleted
	}
	record, err := getSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !record.Deleted {
		return "", fmt.Errorf("session %s is not deleted in the store; retry ClearSession to recover blocked persistence", sessionID)
	}
	return s.RestoreSession(ctx, lifecyclecommand.RestoreSession{SessionID: sessionID})
}

func (s LifecycleService) ClearCurrent(ctx context.Context) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	return s.ClearSession(ctx, s.Memory.CurrentSessionId())
}

func (s LifecycleService) ClearSession(ctx context.Context, sessionID string) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	current, err := s.Memory.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	clearer, targeted := s.Memory.(interface{ ClearSession(string) error })
	if !targeted && s.Memory.CurrentSessionId() != sessionID {
		return nil, errors.New("memory store does not support clearing a non-current session")
	}
	if s.Sessions != nil {
		record, err := s.Sessions.GetSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if record.Deleted {
			return nil, errors.New("cannot clear a deleted session; restore it explicitly first")
		}
	}
	// Keep the captured ID even if another operation switches the current session.
	if s.Messages != nil {
		if err := s.Messages.ClearMessages(ctx, sessionID); err != nil {
			return nil, err
		}
	}
	if targeted {
		err = clearer.ClearSession(sessionID)
	} else {
		err = s.Memory.ClearCurrent()
	}
	if err != nil {
		return nil, err
	}
	// Production memory stores return snapshots, so fetch the cleared aggregate.
	return s.Memory.GetSession(current.ID)
}

func (s LifecycleService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
