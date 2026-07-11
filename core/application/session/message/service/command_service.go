package service

import (
	"context"
	"errors"
	"strings"

	messageapi "myai/core/application/session/message/api"
	messagecommand "myai/core/application/session/message/command"
	messageport "myai/core/application/session/message/port"
	messageresult "myai/core/application/session/message/result"
	"myai/core/session"
)

type CommandService struct {
	// 消息命令只修改内存聚合根；异步落库由 generation adapter 在外层完成。
	Loader messageport.SessionLoader
	Memory messageport.CommandMemory
}

var _ messageapi.CommandService = CommandService{}

func (s CommandService) AppendUserMessage(ctx context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error) {
	if strings.TrimSpace(command.Input) == "" {
		return messageresult.Command{}, errors.New("input is empty")
	}
	current, err := s.loadSession(ctx, command.SessionID)
	if err != nil {
		return messageresult.Command{}, err
	}
	if err := s.Memory.AddUserMessageTo(current.ID, command.Input); err != nil {
		return messageresult.Command{}, err
	}
	current, err = s.Memory.GetSession(current.ID)
	if err != nil {
		return messageresult.Command{}, err
	}
	return messageresult.Command{Session: current, Input: command.Input}, nil
}

func (s CommandService) PrepareRegeneration(ctx context.Context, command messagecommand.PrepareRegeneration) (messageresult.Command, error) {
	current, err := s.loadSession(ctx, command.SessionID)
	if err != nil {
		return messageresult.Command{}, err
	}
	// 重新生成会删除最后一条 user 之后的 assistant/tool 消息，再用同一输入调用模型。
	input, err := s.Memory.TrimAfterLastUserMessage(current.ID)
	if err != nil {
		return messageresult.Command{}, err
	}
	current, err = s.Memory.GetSession(current.ID)
	if err != nil {
		return messageresult.Command{}, err
	}
	return messageresult.Command{Session: current, Input: input}, nil
}

func (s CommandService) loadSession(ctx context.Context, sessionID string) (*session.Session, error) {
	if s.Memory == nil {
		return nil, errors.New("session manager is nil")
	}
	if s.Loader == nil {
		return nil, errors.New("session loader is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = s.Memory.CurrentSessionId()
	}
	if sessionID == "" {
		return nil, errors.New("session id is empty")
	}
	return s.Loader.Load(ctx, sessionID)
}
