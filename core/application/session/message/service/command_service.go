package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	messageapi "myai/core/application/session/message/api"
	messagecommand "myai/core/application/session/message/command"
	messageport "myai/core/application/session/message/port"
	messageresult "myai/core/application/session/message/result"
	domainmessage "myai/core/domain/message"
	"myai/core/session"
)

type CommandService struct {
	// 消息命令只修改内存聚合根；异步落库由 generation adapter 在外层完成。
	Loader              messageport.SessionLoader
	Memory              messageport.CommandMemory
	RuntimeInstructions messageport.RuntimeInstructionProvider
	Regeneration        messageport.RegenerationPersistence
}

var _ messageapi.CommandService = CommandService{}

func (s CommandService) AppendUserMessage(ctx context.Context, command messagecommand.AppendUserMessage) (messageresult.Command, error) {
	if strings.TrimSpace(command.Input) == "" {
		return messageresult.Command{}, errors.New("input is empty")
	}
	command.SourceID = strings.TrimSpace(command.SourceID)
	current, err := s.loadSession(ctx, command.SessionID)
	if err != nil {
		return messageresult.Command{}, err
	}
	// 1. 只有调用方提供稳定 SourceID 时才执行幂等判断；普通消息不能按文本猜测重复。
	if command.SourceID != "" {
		for _, message := range current.Messages {
			if message.SourceID != command.SourceID {
				continue
			}
			// 2. 同一个事件再次投递且内容一致，表示已经应用，直接返回成功。
			if message.Text() == command.Input && (command.SourceKind == "" || message.SourceKind == command.SourceKind) {
				return messageresult.Command{Session: current, Input: command.Input, Appended: false}, nil
			}
			// 3. 同一个事件 ID 携带不同内容，说明上游协议发生冲突，不能静默覆盖。
			return messageresult.Command{}, fmt.Errorf("source message id %q conflicts with existing session message", command.SourceID)
		}
	}
	runtimeInstruction := s.runtimeInstruction(ctx, current, command.Input, command.ForceChatMode, command.ForceAutonomousPlanning)
	messageCount := len(current.Messages)
	// 4. 追加时把事件来源一起写入会话聚合根，后续重试和进程恢复才能继续按 ID 判断。
	if err := s.Memory.AddUserTurnWithMetadataTo(current.ID, command.RAGContext, runtimeInstruction, command.Input, command.SyntheticReason, command.SourceID, command.SourceKind, command.SourceTaskID); err != nil {
		return messageresult.Command{}, err
	}
	current, err = s.Memory.GetSession(current.ID)
	if err != nil {
		return messageresult.Command{}, err
	}
	appended := domainmessage.CloneAll(current.Messages[messageCount:])
	return messageresult.Command{Session: current, Input: command.Input, RuntimeInstruction: runtimeInstruction, RAGContext: strings.TrimSpace(command.RAGContext), AppendedMessages: appended, Appended: true}, nil
}

func (s CommandService) PrepareRegeneration(ctx context.Context, command messagecommand.PrepareRegeneration) (messageresult.Command, error) {
	current, err := s.loadSession(ctx, command.SessionID)
	if err != nil {
		return messageresult.Command{}, err
	}
	beforeRegeneration := session.Clone(current)
	// 重新生成会删除最后一条 user 之后的 assistant/tool 消息，再用同一输入调用模型。
	input, err := s.Memory.TrimAfterLastUserMessage(current.ID)
	if err != nil {
		return messageresult.Command{}, err
	}
	current, err = s.Memory.GetSession(current.ID)
	if err != nil {
		return messageresult.Command{}, err
	}
	if s.Regeneration != nil {
		if err := s.Regeneration.PersistRegeneratedSession(ctx, current); err != nil {
			if restoreErr := s.Memory.RestoreSession(beforeRegeneration); restoreErr != nil {
				return messageresult.Command{}, errors.Join(err, fmt.Errorf("restore session after regeneration persistence failure: %w", restoreErr))
			}
			return messageresult.Command{}, err
		}
	}
	return messageresult.Command{Session: current, Input: input, RuntimeInstruction: latestRuntimeInstruction(current.Messages)}, nil
}

func (s CommandService) runtimeInstruction(ctx context.Context, current *session.Session, input string, forceChatMode bool, forceAutonomousPlanning bool) string {
	if s.RuntimeInstructions == nil {
		return ""
	}
	if forceAutonomousPlanning {
		if provider, ok := s.RuntimeInstructions.(messageport.PlanningRuntimeInstructionProvider); ok {
			return strings.TrimSpace(provider.PromptForMode(ctx, current, input, forceChatMode, true))
		}
	}
	return strings.TrimSpace(s.RuntimeInstructions.Prompt(ctx, current, input, forceChatMode))
}

func latestRuntimeInstruction(messages []domainmessage.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != domainmessage.RoleUser {
			continue
		}
		for previous := index - 1; previous >= 0 && messages[previous].IsSynthetic(); previous-- {
			if instruction, ok := messages[previous].RuntimeInstructionText(); ok {
				return instruction
			}
		}
		return ""
	}
	return ""
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
