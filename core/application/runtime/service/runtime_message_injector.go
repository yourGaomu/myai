package service

import (
	"strings"

	domainmessage "myai/core/domain/message"
)

const RuntimeInstructionPrefix = "Runtime instructions for this turn:"

func InsertRuntimeInstructions(messages []domainmessage.Message, runtimePrompt string) []domainmessage.Message {
	prompt := strings.TrimSpace(runtimePrompt)
	if prompt == "" {
		return messages
	}

	// 运行时指令只存在于本次快照副本中。插在最新 user 前面，可以保留固定 system 与历史消息前缀，
	// 同时保证模型在读取本轮输入前先看到 Plan/Skill 规则。
	withRuntime := make([]domainmessage.Message, 0, len(messages)+1)
	insertAt := len(messages)
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == domainmessage.RoleUser {
			insertAt = index
			break
		}
	}

	withRuntime = append(withRuntime, messages[:insertAt]...)
	withRuntime = append(withRuntime, domainmessage.Text(domainmessage.RoleSystem, RuntimeInstructionPrefix+"\n"+prompt))
	withRuntime = append(withRuntime, messages[insertAt:]...)
	return withRuntime
}
