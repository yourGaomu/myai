package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"myai/core/domain/generation"
	domaintool "myai/core/domain/tool"
	"myai/core/llm"
	"myai/core/service"
	tooldef "myai/core/tool/tool"
)

// ChatFacade 定义 OneBot 模块与底层 ChatService 交互所需的窄接口（遵循依赖倒置原则）。
// 1.1 会话生命周期与对话生成：CreateSession / SendMessageStreamForSession / DeleteSession；
// 1.2 会话级配置与权限：SetPermissionModeForSession / SetStyleInstructionForSession / SetGenerationSettingsForSession / SessionPreferencesForSession；
// 1.3 模型目录与切换：ListModels / SwitchModelForSession / CurrentModelID；
// 1.4 上下文状态与压缩：ContextInfoForSession / CompactSession。
type ChatFacade interface {
	CreateSession(ctx context.Context, title string) (string, error)
	SendMessageStreamForSession(ctx context.Context, sessionID string, input string, stream llm.ChatStreamHandler) (service.ChatResponse, error)
	DeleteSession(ctx context.Context, sessionID string) error
	SetPermissionModeForSession(ctx context.Context, sessionID string, mode string) error
	SetStyleInstructionForSession(ctx context.Context, sessionID string, instruction string) error
	SetGenerationSettingsForSession(ctx context.Context, sessionID string, settings generation.Settings) error
	SessionPreferencesForSession(ctx context.Context, sessionID string) (service.SessionPreferencesView, error)
	ListModels() []llm.ModelInfo
	SwitchModelForSession(ctx context.Context, sessionID string, modelID string) error
	CurrentModelID() string
	ContextInfoForSession(ctx context.Context, sessionID string) (service.ContextInfo, error)
	CompactSession(ctx context.Context, sessionID string) (service.ContextInfo, error)
}

// BotRuntimeStatus 汇总机器人当前的实时连接状态与运行指标，供 onebot_get_status 工具返回。
// 2.1 Connected：当前与 NapCatQQ 的正向 WebSocket 是否处于已连接状态；
// 2.2 WSURL / SelfID：连接的 WebSocket 地址与机器人自身 QQ 号；
// 2.3 LastHeartbeatLatencyMS：最近一次心跳或通信延迟（毫秒）；
// 2.4 StartedAt / UptimeSeconds：进程启动时间与已运行秒数。
type BotRuntimeStatus struct {
	Connected              bool      `json:"connected"`
	WSURL                  string    `json:"ws_url"`
	SelfID                 int64     `json:"self_id"`
	LastHeartbeatLatencyMS int64     `json:"last_heartbeat_latency_ms"`
	StartedAt              time.Time `json:"started_at"`
	UptimeSeconds          int64     `json:"uptime_seconds"`
}

// MessageSender 定义主动通过 OneBot WebSocket 发送消息与查询运行状态的抽象接口。
// 2.1 SendTextMessage：向指定 QQ 用户（private）或 QQ 群（group）主动推送纯文本消息；
// 2.2 RuntimeStatus：获取当前 WebSocket 连接状态与运行时长。
type MessageSender interface {
	SendTextMessage(ctx context.Context, messageType string, targetID int64, content string) error
	RuntimeStatus() BotRuntimeStatus
}

// NewTools 创建并返回全部 9 个全自然语言驱动的 OneBot 专属管理工具。
// 3.1 6.1 onebot_set_user_role：任命/罢免管理员与拉黑用户（细粒度目标角色鉴权）；
// 3.2 6.2 onebot_clear_history：清空私聊或群聊上下文记忆；
// 3.3 6.3 onebot_set_group_switch：开启或关闭指定群的机器人服务；
// 3.4 6.4 onebot_set_persona：动态设置机器人说话风格/人设；
// 3.5 6.5 onebot_switch_model：查看可用模型、切换模型及调整推理参数（temperature/max_tokens/top_p）；
// 3.6 6.6 onebot_list_users_and_sessions：查询管理员名单、用户身份与群状态；
// 3.7 6.7 onebot_send_message：主动向指定 QQ 好友或群聊发送消息（内置私聊白名单防 SPAM）；
// 3.8 6.8 onebot_get_status：查询机器人当前连接状态、模型与统计信息；
// 3.9 6.9 onebot_query：自然语言驱动的内部数据只读 SQL 查询工具（Text2SQL 白名单安全执行）。
func NewTools(store Store, chat ChatFacade, sender MessageSender) []tooldef.Tool {
	return []tooldef.Tool{
		NewSetUserRoleTool(store),
		NewClearHistoryTool(store, chat),
		NewSetGroupSwitchTool(store),
		NewSetPersonaTool(store, chat),
		NewSwitchModelTool(store, chat),
		NewListUsersAndSessionsTool(store),
		NewSendMessageTool(store, sender),
		NewGetStatusTool(store, chat, sender),
		NewQueryTool(store),
	}
}

// authorizeCaller 从 Go Context 中提取不可篡改的真实发送者 QQ 号，并实时查表校验最低角色权限。
// 4.1 从 ctx 中提取 CallerInfo，若不存在或 UserID <= 0 则拒绝执行（防范脱离 OneBot 上下文的非法调用）；
// 4.2 实时查询 `onebot_users` 表获取该 QQ 号当前的最新 Role（若未入库则视为普通用户 RoleUser）；
// 4.3 对比用户实际角色与 requiredRole，权限不足时返回明确的 permission_denied 错误。
func authorizeCaller(ctx context.Context, store Store, requiredRole Role) (CallerInfo, UserIdentity, error) {
	// 4.1 提取底层事件注入的真实调用者信息
	caller, ok := CallerFromContext(ctx)
	if !ok || caller.UserID <= 0 {
		return CallerInfo{}, UserIdentity{}, errors.New("未检测到合法的 OneBot 调用者上下文")
	}
	if store == nil {
		return CallerInfo{}, UserIdentity{}, errors.New("OneBot 存储未初始化")
	}

	// 4.2 实时查表获取调用者当前角色
	identity, found, err := store.GetUser(ctx, caller.UserID)
	if err != nil {
		return CallerInfo{}, UserIdentity{}, fmt.Errorf("查询调用者权限失败: %w", err)
	}
	if !found {
		identity = UserIdentity{
			UserID:   caller.UserID,
			Nickname: caller.Nickname,
			Role:     RoleUser,
		}
	}

	// 4.3 校验角色等级是否达标
	if !HasAtLeastRole(identity.Role, requiredRole) {
		return caller, identity, fmt.Errorf("当前操作需要 %s 及以上权限，您的当前身份为 %s (QQ: %d)",
			requiredRole, identity.Role, caller.UserID)
	}
	return caller, identity, nil
}

// permissionDeniedOutput 构造统一的权限拒绝 ToolOutput。
func permissionDeniedOutput(err error) tooldef.ToolOutput {
	return tooldef.FailedOutput(domaintool.ResultStatusFailed, "permission_denied", err.Error())
}

// invalidInputOutput 构造统一的参数校验失败 ToolOutput。
func invalidInputOutput(message string) tooldef.ToolOutput {
	return tooldef.FailedOutput(domaintool.ResultStatusFailed, "invalid_input", message)
}

// executionFailedOutput 构造统一的内部执行失败 ToolOutput。
func executionFailedOutput(code string, err error) tooldef.ToolOutput {
	return tooldef.FailedOutput(domaintool.ResultStatusFailed, code, err.Error())
}

// jsonSuccessOutput 将任意结构体序列化为格式化 JSON 并包装为成功的 ToolOutput。
func jsonSuccessOutput(payload any) (tooldef.ToolOutput, error) {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return executionFailedOutput("marshal_failed", err), nil
	}
	return tooldef.SuccessOutput(string(data)), nil
}

// resolveScopedSession 根据 scope ("current" | "private" | "group") 和 targetID 解析或创建目标会话。
// 5.1 规范化 scope，为空时默认为 "current"；
// 5.2 若 scope == "current"，根据 caller.MessageType 自动映射为当前群聊或当前私聊；
// 5.3 若为私聊目标（"private"）：查 `onebot_users` 表获取 PrivateSessionID，若 createIfMissing 为 true 且为空则调用 chat.CreateSession 创建并回写；
// 5.4 若为群聊目标（"group"）：查 `onebot_groups` 表获取 SessionID，若 createIfMissing 为 true 且为空则调用 chat.CreateSession 创建并回写。
func resolveScopedSession(
	ctx context.Context,
	store Store,
	chat ChatFacade,
	caller CallerInfo,
	scope string,
	targetID int64,
	createIfMissing bool,
) (sessionID string, resolvedScope string, resolvedID int64, err error) {
	// 5.1 规范化 scope 参数
	sc := strings.ToLower(strings.TrimSpace(scope))
	if sc == "" {
		sc = "current"
	}

	// 5.2 将 "current" 展开为当前实际所在的群聊或私聊作用域
	if sc == "current" {
		if caller.MessageType == "group" && caller.GroupID > 0 {
			sc = "group"
			if targetID <= 0 {
				targetID = caller.GroupID
			}
			if caller.SessionID != "" && targetID == caller.GroupID {
				return caller.SessionID, "group", caller.GroupID, nil
			}
		} else {
			sc = "private"
			if targetID <= 0 {
				targetID = caller.UserID
			}
			if caller.SessionID != "" && targetID == caller.UserID {
				return caller.SessionID, "private", caller.UserID, nil
			}
		}
	}

	switch sc {
	case "private":
		// 5.3 解析私聊目标会话
		if targetID <= 0 {
			targetID = caller.UserID
		}
		if targetID <= 0 {
			return "", "private", 0, errors.New("未指定目标私聊 QQ 号")
		}
		user, found, getErr := store.GetUser(ctx, targetID)
		if getErr != nil {
			return "", "private", targetID, getErr
		}
		if !found {
			user = UserIdentity{
				UserID: targetID,
				Role:   RoleUser,
			}
		}
		if strings.TrimSpace(user.PrivateSessionID) == "" && createIfMissing && chat != nil {
			newID, createErr := chat.CreateSession(ctx, fmt.Sprintf("QQ私聊:%d", targetID))
			if createErr != nil {
				return "", "private", targetID, createErr
			}
			user.PrivateSessionID = newID
			user.UpdatedAt = time.Now().UTC()
			if upErr := store.UpsertUser(ctx, user); upErr != nil {
				return "", "private", targetID, upErr
			}
		}
		return strings.TrimSpace(user.PrivateSessionID), "private", targetID, nil

	case "group":
		// 5.4 解析群聊目标会话
		if targetID <= 0 {
			targetID = caller.GroupID
		}
		if targetID <= 0 {
			return "", "group", 0, errors.New("未指定目标 QQ 群号")
		}
		group, found, getErr := store.GetGroup(ctx, targetID)
		if getErr != nil {
			return "", "group", targetID, getErr
		}
		if !found {
			group = GroupConfig{
				GroupID: targetID,
				Enabled: true,
			}
		}
		if strings.TrimSpace(group.SessionID) == "" && createIfMissing && chat != nil {
			newID, createErr := chat.CreateSession(ctx, fmt.Sprintf("QQ群聊:%d", targetID))
			if createErr != nil {
				return "", "group", targetID, createErr
			}
			group.SessionID = newID
			group.UpdatedBy = caller.UserID
			group.UpdatedAt = time.Now().UTC()
			if upErr := store.UpsertGroup(ctx, group); upErr != nil {
				return "", "group", targetID, upErr
			}
		}
		return strings.TrimSpace(group.SessionID), "group", targetID, nil

	default:
		return "", sc, targetID, fmt.Errorf("不支持的 scope %q，仅支持 current / private / group", scope)
	}
}
