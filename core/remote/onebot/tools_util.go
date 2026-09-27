package onebot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tooldef "myai/core/tool/tool"
)

// ============================================================================
// 6.6 onebot_list_users_and_sessions — 查询管理员名单、用户身份与群状态
// ============================================================================

// ListUsersAndSessionsTool 实现 `onebot_list_users_and_sessions` 工具。
type ListUsersAndSessionsTool struct {
	store Store
}

type listUsersArgs struct {
	RoleFilter string `json:"role_filter,omitempty"`
}

// NewListUsersAndSessionsTool 创建 `onebot_list_users_and_sessions` 工具实例。
func NewListUsersAndSessionsTool(store Store) *ListUsersAndSessionsTool {
	return &ListUsersAndSessionsTool{store: store}
}

func (t *ListUsersAndSessionsTool) Name() string {
	return "onebot_list_users_and_sessions"
}

func (t *ListUsersAndSessionsTool) Description() string {
	return "查询机器人已记录的 QQ 用户身份列表（可按 super_admin / admin / user / banned 过滤）以及各 QQ 群的服务开关与人设状态（需要 admin 或 super_admin 权限）。"
}

func (t *ListUsersAndSessionsTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"role_filter": map[string]any{
				"type":        "string",
				"enum":        []string{"all", "super_admin", "admin", "user", "banned"},
				"description": "按角色过滤用户列表，默认 all",
			},
		},
	}
}

func (t *ListUsersAndSessionsTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// Call 执行用户与群状态列表查询。
// 1.1 实时查表校验调用者是否为 admin 或 super_admin；
// 1.2 解析 role_filter 参数并查询 `onebot_users` 表；
// 1.3 查询 `onebot_groups` 表获取所有群聊开关与人设配置；
// 1.4 汇总返回结构化 JSON 数据。
func (t *ListUsersAndSessionsTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 校验管理员权限
	if _, _, err := authorizeCaller(ctx, t.store, RoleAdmin); err != nil {
		return permissionDeniedOutput(err), nil
	}

	var args listUsersArgs
	if len(rawArgs) > 0 && string(rawArgs) != "null" {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return invalidInputOutput("解析参数失败: " + err.Error()), nil
		}
	}
	filter := strings.ToLower(strings.TrimSpace(args.RoleFilter))
	if filter == "" {
		filter = "all"
	}

	// 1.2 查询用户列表
	users, err := t.store.ListUsers(ctx, filter)
	if err != nil {
		return executionFailedOutput("db_error", err), nil
	}

	// 1.3 查询群配置列表
	groups, err := t.store.ListGroups(ctx)
	if err != nil {
		return executionFailedOutput("db_error", err), nil
	}

	// 1.4 组装返回结果
	return jsonSuccessOutput(map[string]any{
		"role_filter": filter,
		"user_count":  len(users),
		"users":       users,
		"group_count": len(groups),
		"groups":      groups,
	})
}

// ============================================================================
// 6.7 onebot_send_message — 主动向指定 QQ 好友或群聊发送消息（含 SPAM 防护）
// ============================================================================

// SendMessageTool 实现 `onebot_send_message` 工具，支持主动向指定群或已建立会话的用户发送消息。
type SendMessageTool struct {
	store  Store
	sender MessageSender
}

type sendMessageArgs struct {
	MessageType string `json:"message_type"`
	TargetID    int64  `json:"target_id"`
	Content     string `json:"content"`
}

// NewSendMessageTool 创建 `onebot_send_message` 工具实例。
func NewSendMessageTool(store Store, sender MessageSender) *SendMessageTool {
	return &SendMessageTool{store: store, sender: sender}
}

func (t *SendMessageTool) Name() string {
	return "onebot_send_message"
}

func (t *SendMessageTool) Description() string {
	return "主动向指定 QQ 用户（private）或 QQ 群（group）发送文本消息（需要 admin 或 super_admin 权限）。注意：私聊仅允许发送给已存在于 onebot_users 表中的用户以防范 SPAM。"
}

func (t *SendMessageTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_type": map[string]any{
				"type":        "string",
				"enum":        []string{"private", "group"},
				"description": "消息类型：private（私聊）或 group（群聊）",
			},
			"target_id": map[string]any{
				"type":        "integer",
				"description": "目标 QQ 号（当 message_type=private 时）或目标群号（当 message_type=group 时）",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要发送的纯文本消息内容",
			},
		},
		"required": []string{"message_type", "target_id", "content"},
	}
}

func (t *SendMessageTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// Call 执行主动消息发送。
// 1.1 实时查表校验调用者是否为 admin 或 super_admin；
// 1.2 校验 message_type、target_id 与 content 参数合法性；
// 1.3 SPAM 防护：当 message_type == "private" 时，查 `onebot_users` 表确认 target_id 已存在（曾主动与机器人交互过或已被登记），否则拒绝发送；
// 1.4 调用 sender.SendTextMessage 通过正向 WebSocket 下发 `send_msg` 动作。
func (t *SendMessageTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 校验管理员权限
	if _, _, err := authorizeCaller(ctx, t.store, RoleAdmin); err != nil {
		return permissionDeniedOutput(err), nil
	}
	if t.sender == nil {
		return executionFailedOutput("sender_unavailable", fmt.Errorf("onebot message sender is nil")), nil
	}

	// 1.2 解析并校验参数
	var args sendMessageArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return invalidInputOutput("解析参数失败: " + err.Error()), nil
	}
	msgType := strings.ToLower(strings.TrimSpace(args.MessageType))
	if msgType != "private" && msgType != "group" {
		return invalidInputOutput("message_type 仅支持 private 或 group"), nil
	}
	if args.TargetID <= 0 {
		return invalidInputOutput("target_id 必须为大于 0 的有效 QQ 号或群号"), nil
	}
	content := strings.TrimSpace(args.Content)
	if content == "" {
		return invalidInputOutput("content 消息内容不能为空"), nil
	}

	// 1.3 SPAM 防护：私聊目标必须已存在于 onebot_users 表中
	if msgType == "private" {
		_, found, err := t.store.GetUser(ctx, args.TargetID)
		if err != nil {
			return executionFailedOutput("db_error", err), nil
		}
		if !found {
			return invalidInputOutput("目标用户未与机器人建立过会话，禁止主动发起私聊以防范垃圾消息"), nil
		}
	}

	// 1.4 通过 WebSocket 发送消息
	if err := t.sender.SendTextMessage(ctx, msgType, args.TargetID, content); err != nil {
		return executionFailedOutput("send_failed", err), nil
	}

	return jsonSuccessOutput(map[string]any{
		"message":      fmt.Sprintf("已成功向 %s (%d) 发送消息", msgType, args.TargetID),
		"message_type": msgType,
		"target_id":    args.TargetID,
	})
}

// ============================================================================
// 6.8 onebot_get_status — 查询机器人当前运行状态
// ============================================================================

// GetStatusTool 实现 `onebot_get_status` 工具，汇总连接状态、当前模型及用户/群统计指标。
type GetStatusTool struct {
	store  Store
	chat   ChatFacade
	sender MessageSender
}

// NewGetStatusTool 创建 `onebot_get_status` 工具实例。
func NewGetStatusTool(store Store, chat ChatFacade, sender MessageSender) *GetStatusTool {
	return &GetStatusTool{store: store, chat: chat, sender: sender}
}

func (t *GetStatusTool) Name() string {
	return "onebot_get_status"
}

func (t *GetStatusTool) Description() string {
	return "查询机器人当前的运行状态，包括 NapCatQQ WebSocket 连接状态与延迟、当前使用的 LLM 模型、已注册用户统计（总数/管理员/封禁）以及群聊启用统计（需要 admin 或 super_admin 权限）。"
}

func (t *GetStatusTool) Schema() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *GetStatusTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// Call 执行运行状态汇总查询。
// 1.1 实时查表校验调用者是否为 admin 或 super_admin；
// 1.2 从 sender 获取 WebSocket 连接状态、SelfID、心跳延迟与运行时长；
// 1.3 统计 `onebot_users` 表中各角色的用户数量；
// 1.4 统计 `onebot_groups` 表中已开启与总群聊数量并返回 JSON。
func (t *GetStatusTool) Call(ctx context.Context, _ json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 校验管理员权限
	if _, _, err := authorizeCaller(ctx, t.store, RoleAdmin); err != nil {
		return permissionDeniedOutput(err), nil
	}

	// 1.2 获取 WebSocket 与进程运行时状态
	var runtimeStatus BotRuntimeStatus
	if t.sender != nil {
		runtimeStatus = t.sender.RuntimeStatus()
	}
	currentModel := ""
	if t.chat != nil {
		currentModel = t.chat.CurrentModelID()
	}

	// 1.3 统计用户角色分布
	users, err := t.store.ListUsers(ctx, "all")
	if err != nil {
		return executionFailedOutput("db_error", err), nil
	}
	superAdminCount := 0
	adminCount := 0
	normalUserCount := 0
	bannedCount := 0
	for _, u := range users {
		switch u.Role {
		case RoleSuperAdmin:
			superAdminCount++
		case RoleAdmin:
			adminCount++
		case RoleBanned:
			bannedCount++
		default:
			normalUserCount++
		}
	}

	// 1.4 统计群聊开启情况
	groups, err := t.store.ListGroups(ctx)
	if err != nil {
		return executionFailedOutput("db_error", err), nil
	}
	enabledGroups := 0
	for _, g := range groups {
		if g.Enabled {
			enabledGroups++
		}
	}

	return jsonSuccessOutput(map[string]any{
		"websocket":        runtimeStatus,
		"current_model_id": currentModel,
		"users": map[string]any{
			"total":       len(users),
			"super_admin": superAdminCount,
			"admin":       adminCount,
			"user":        normalUserCount,
			"banned":      bannedCount,
		},
		"groups": map[string]any{
			"total":    len(groups),
			"enabled":  enabledGroups,
			"disabled": len(groups) - enabledGroups,
		},
	})
}
