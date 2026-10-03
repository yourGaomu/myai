package onebot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"myai/core/domain/generation"
	tooldef "myai/core/tool/tool"
)

// ============================================================================
// 6.1 onebot_set_user_role — 任命/罢免管理员与拉黑用户
// ============================================================================

// SetUserRoleTool 实现 `onebot_set_user_role` 工具，支持按目标角色细分的动态权限管理。
type SetUserRoleTool struct {
	store Store
}

type setUserRoleArgs struct {
	TargetQQ int64  `json:"target_qq"`
	Role     string `json:"role"`
	Nickname string `json:"nickname,omitempty"`
}

// NewSetUserRoleTool 创建 `onebot_set_user_role` 工具实例。
func NewSetUserRoleTool(store Store) *SetUserRoleTool {
	return &SetUserRoleTool{store: store}
}

func (t *SetUserRoleTool) Name() string {
	return "onebot_set_user_role"
}

func (t *SetUserRoleTool) Description() string {
	return "设置指定 QQ 用户的机器人权限角色（admin 管理员 / user 普通用户 / banned 黑名单封禁）。任命或罢免 admin 仅限 super_admin，封禁或解封普通用户允许 admin 或 super_admin 执行。"
}

func (t *SetUserRoleTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target_qq": map[string]any{
				"type":        "integer",
				"description": "目标用户的 QQ 号（若消息中包含 [提及用户 QQ: xxx]，请直接提取该数字）",
			},
			"role": map[string]any{
				"type":        "string",
				"enum":        []string{"admin", "user", "banned"},
				"description": "要设置的目标角色：admin（管理员）、user（普通用户）、banned（黑名单封禁）",
			},
			"nickname": map[string]any{
				"type":        "string",
				"description": "可选的目标用户昵称备注",
			},
		},
		"required": []string{"target_qq", "role"},
	}
}

func (t *SetUserRoleTool) Permission() tooldef.Permission {
	// 设为 PermissionRead 以便在群聊 Readonly 模式下可被大模型触发，真实安全边界由 Call 内部实时查表鉴权保障。
	return tooldef.PermissionRead
}

// Call 执行角色变更操作。
// 1.1 解析并校验输入参数 target_qq 与目标 role（仅允许 admin / user / banned）；
// 1.2 根据目标角色决定所需最低调用者权限：
//   - 若目标 role == "admin"，要求调用者必须是 super_admin；
//   - 若目标 role == "user" 或 "banned"，要求调用者至少是 admin；
//
// 1.3 查询目标用户当前在 `onebot_users` 表中的既有身份：
//   - 若目标用户当前是 super_admin，禁止任何人通过工具修改其角色；
//   - 若目标用户当前是 admin（即准备罢免或封禁管理员），要求调用者必须是 super_admin；
//
// 1.4 写入 `onebot_users` 表，记录 granted_by = caller.UserID 并返回结果。
func (t *SetUserRoleTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 解析并校验参数
	var args setUserRoleArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return invalidInputOutput("解析参数失败: " + err.Error()), nil
	}
	if args.TargetQQ <= 0 {
		return invalidInputOutput("target_qq 必须为大于 0 的有效 QQ 号"), nil
	}
	targetRole := Role(strings.ToLower(strings.TrimSpace(args.Role)))
	if targetRole != RoleAdmin && targetRole != RoleUser && targetRole != RoleBanned {
		return invalidInputOutput("role 仅支持 admin、user 或 banned"), nil
	}

	// 1.2 按目标角色确定调用者所需的最低角色等级
	requiredCallerRole := RoleAdmin
	if targetRole == RoleAdmin {
		requiredCallerRole = RoleSuperAdmin
	}
	caller, callerIdentity, err := authorizeCaller(ctx, t.store, requiredCallerRole)
	if err != nil {
		return permissionDeniedOutput(err), nil
	}

	// 1.3 查询目标用户的既有身份，防止越权修改 super_admin 或同级 admin
	existing, found, err := t.store.GetUser(ctx, args.TargetQQ)
	if err != nil {
		return executionFailedOutput("db_error", err), nil
	}
	if found {
		if existing.Role == RoleSuperAdmin {
			return permissionDeniedOutput(fmt.Errorf("目标用户 %d 是超级管理员(super_admin)，禁止通过工具修改或剥夺其权限", args.TargetQQ)), nil
		}
		if existing.Role == RoleAdmin && callerIdentity.Role != RoleSuperAdmin {
			return permissionDeniedOutput(fmt.Errorf("目标用户 %d 当前是管理员(admin)，仅超级管理员(super_admin)有权罢免或封禁管理员", args.TargetQQ)), nil
		}
	}

	// 1.4 构造更新记录并持久化到 onebot_users 表
	now := time.Now().UTC()
	nick := strings.TrimSpace(args.Nickname)
	if nick == "" && found {
		nick = existing.Nickname
	}
	updated := UserIdentity{
		UserID:           args.TargetQQ,
		Nickname:         nick,
		Role:             targetRole,
		GrantedBy:        caller.UserID,
		PrivateSessionID: existing.PrivateSessionID,
		LastActiveAt:     existing.LastActiveAt,
		CreatedAt:        existing.CreatedAt,
		UpdatedAt:        now,
	}
	if err := t.store.UpsertUser(ctx, updated); err != nil {
		return executionFailedOutput("db_error", err), nil
	}

	return jsonSuccessOutput(map[string]any{
		"message":    fmt.Sprintf("已成功将用户 %d 的角色设置为 %s", args.TargetQQ, targetRole),
		"target_qq":  args.TargetQQ,
		"role":       targetRole,
		"granted_by": caller.UserID,
	})
}

// ============================================================================
// 6.2 onebot_clear_history — 清空私聊或群聊上下文记忆
// ============================================================================

// ClearHistoryTool 实现 `onebot_clear_history` 工具，用于重置指定私聊或群聊的对话上下文。
type ClearHistoryTool struct {
	store Store
	chat  ChatFacade
}

type clearHistoryArgs struct {
	Scope    string `json:"scope,omitempty"`
	TargetID int64  `json:"target_id,omitempty"`
}

// NewClearHistoryTool 创建 `onebot_clear_history` 工具实例。
func NewClearHistoryTool(store Store, chat ChatFacade) *ClearHistoryTool {
	return &ClearHistoryTool{store: store, chat: chat}
}

func (t *ClearHistoryTool) Name() string {
	return "onebot_clear_history"
}

func (t *ClearHistoryTool) Description() string {
	return "清空并重置当前会话、指定 QQ 用户私聊或指定 QQ 群聊的历史对话上下文（需要 admin 或 super_admin 权限）。"
}

func (t *ClearHistoryTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{"current", "private", "group"},
				"description": "清空范围：current（当前会话，默认）、private（指定用户私聊）、group（指定群聊）",
			},
			"target_id": map[string]any{
				"type":        "integer",
				"description": "当 scope 为 private 或 group 时，指定目标 QQ 号或群号；不填则默认取当前发送者或当前群",
			},
		},
	}
}

func (t *ClearHistoryTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// Call 执行会话历史清空操作。
// 1.1 实时查表校验调用者是否为 admin 或 super_admin；
// 1.2 解析 scope 与 target_id 获取对应的 SessionID；
// 1.3 若存在绑定的 SessionID，调用 chat.DeleteSession 删除旧会话；
// 1.4 将 `onebot_users.private_session_id` 或 `onebot_groups.session_id` 置空，下次对话时自动分配全新会话。
func (t *ClearHistoryTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 校验管理员权限
	caller, _, err := authorizeCaller(ctx, t.store, RoleAdmin)
	if err != nil {
		return permissionDeniedOutput(err), nil
	}

	var args clearHistoryArgs
	if len(rawArgs) > 0 && string(rawArgs) != "null" {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return invalidInputOutput("解析参数失败: " + err.Error()), nil
		}
	}

	// 1.2 定位目标会话
	sessionID, resolvedScope, resolvedID, err := resolveScopedSession(ctx, t.store, t.chat, caller, args.Scope, args.TargetID, false)
	if err != nil {
		return invalidInputOutput(err.Error()), nil
	}

	// 1.3 调用 ChatService 删除旧会话（若会话在内存或库中不存在则忽略不存在错误）
	if sessionID != "" && t.chat != nil {
		_ = t.chat.DeleteSession(ctx, sessionID)
	}

	// 1.4 将数据库表中的会话映射字段置空
	now := time.Now().UTC()
	if resolvedScope == "group" {
		group, found, getErr := t.store.GetGroup(ctx, resolvedID)
		if getErr != nil {
			return executionFailedOutput("db_error", getErr), nil
		}
		if found {
			group.SessionID = ""
			group.UpdatedBy = caller.UserID
			group.UpdatedAt = now
			if upErr := t.store.UpsertGroup(ctx, group); upErr != nil {
				return executionFailedOutput("db_error", upErr), nil
			}
		}
	} else {
		user, found, getErr := t.store.GetUser(ctx, resolvedID)
		if getErr != nil {
			return executionFailedOutput("db_error", getErr), nil
		}
		if found {
			user.PrivateSessionID = ""
			user.UpdatedAt = now
			if upErr := t.store.UpsertUser(ctx, user); upErr != nil {
				return executionFailedOutput("db_error", upErr), nil
			}
		}
	}

	return jsonSuccessOutput(map[string]any{
		"message":            fmt.Sprintf("已清空 %s (%d) 的历史对话记录", resolvedScope, resolvedID),
		"scope":              resolvedScope,
		"target_id":          resolvedID,
		"cleared_session_id": sessionID,
	})
}

// ============================================================================
// 6.3 onebot_set_group_switch — 开启或关闭指定群的机器人服务
// ============================================================================

// SetGroupSwitchTool 实现 `onebot_set_group_switch` 工具，控制机器人在指定 QQ 群的启用状态。
type SetGroupSwitchTool struct {
	store Store
}

type setGroupSwitchArgs struct {
	GroupID   int64  `json:"group_id,omitempty"`
	GroupName string `json:"group_name,omitempty"`
	Enabled   bool   `json:"enabled"`
}

// NewSetGroupSwitchTool 创建 `onebot_set_group_switch` 工具实例。
func NewSetGroupSwitchTool(store Store) *SetGroupSwitchTool {
	return &SetGroupSwitchTool{store: store}
}

func (t *SetGroupSwitchTool) Name() string {
	return "onebot_set_group_switch"
}

func (t *SetGroupSwitchTool) Description() string {
	return "开启或关闭指定 QQ 群的机器人回复服务（需要 admin 或 super_admin 权限）。关闭后普通群消息将被忽略，但管理员仍可通过自然语言重新开启。"
}

func (t *SetGroupSwitchTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"group_id": map[string]any{
				"type":        "integer",
				"description": "目标 QQ 群号；不填则默认取当前所在群聊的群号",
			},
			"group_name": map[string]any{
				"type":        "string",
				"description": "可选的群名称备注",
			},
			"enabled": map[string]any{
				"type":        "boolean",
				"description": "true 表示开启本群机器人回复，false 表示关闭本群机器人回复",
			},
		},
		"required": []string{"enabled"},
	}
}

func (t *SetGroupSwitchTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// Call 执行群聊机器人开关变更。
// 1.1 校验调用者是否为 admin 或 super_admin；
// 1.2 解析目标 group_id（未传时默认取 caller.GroupID）；
// 1.3 查询既有群配置并更新 enabled 字段与 updated_by 审计信息。
func (t *SetGroupSwitchTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 校验管理员权限
	caller, _, err := authorizeCaller(ctx, t.store, RoleAdmin)
	if err != nil {
		return permissionDeniedOutput(err), nil
	}

	var args setGroupSwitchArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return invalidInputOutput("解析参数失败: " + err.Error()), nil
	}

	// 1.2 确定目标群号
	groupID := args.GroupID
	if groupID <= 0 {
		groupID = caller.GroupID
	}
	if groupID <= 0 {
		return invalidInputOutput("当前不在群聊中，请明确指定 group_id 参数"), nil
	}

	// 1.3 更新群配置表
	group, found, err := t.store.GetGroup(ctx, groupID)
	if err != nil {
		return executionFailedOutput("db_error", err), nil
	}
	if !found {
		group = GroupConfig{
			GroupID: groupID,
		}
	}
	if name := strings.TrimSpace(args.GroupName); name != "" {
		group.GroupName = name
	}
	group.Enabled = args.Enabled
	group.UpdatedBy = caller.UserID
	group.UpdatedAt = time.Now().UTC()

	if err := t.store.UpsertGroup(ctx, group); err != nil {
		return executionFailedOutput("db_error", err), nil
	}

	statusText := "开启"
	if !args.Enabled {
		statusText = "关闭"
	}
	return jsonSuccessOutput(map[string]any{
		"message":    fmt.Sprintf("已%s群 %d 的机器人回复服务", statusText, groupID),
		"group_id":   groupID,
		"enabled":    args.Enabled,
		"updated_by": caller.UserID,
	})
}

// ============================================================================
// 6.4 onebot_set_persona — 动态设置机器人说话风格 / 人设
// ============================================================================

// SetPersonaTool 实现 `onebot_set_persona` 工具，用于动态修改群聊或私聊的说话风格与人设。
type SetPersonaTool struct {
	store Store
	chat  ChatFacade
}

type setPersonaArgs struct {
	StyleInstruction string `json:"style_instruction"`
	Scope            string `json:"scope,omitempty"`
	TargetID         int64  `json:"target_id,omitempty"`
}

// NewSetPersonaTool 创建 `onebot_set_persona` 工具实例。
func NewSetPersonaTool(store Store, chat ChatFacade) *SetPersonaTool {
	return &SetPersonaTool{store: store, chat: chat}
}

func (t *SetPersonaTool) Name() string {
	return "onebot_set_persona"
}

func (t *SetPersonaTool) Description() string {
	return "动态设置机器人在当前会话、指定群聊或私聊中的说话风格与人设指令（需要 admin 或 super_admin 权限）。传空字符串表示恢复默认风格。"
}

func (t *SetPersonaTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"style_instruction": map[string]any{
				"type":        "string",
				"description": "新的说话风格或人设描述（传空字符串 \"\" 表示恢复默认风格）",
			},
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{"current", "private", "group"},
				"description": "作用范围：current（当前会话，默认）、private（私聊）、group（群聊）",
			},
			"target_id": map[string]any{
				"type":        "integer",
				"description": "可选的目标 QQ 号或群号",
			},
		},
		"required": []string{"style_instruction"},
	}
}

func (t *SetPersonaTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// Call 执行人设指令更新。
// 1.1 校验调用者是否为 admin 或 super_admin；
// 1.2 解析或创建目标会话 SessionID；
// 1.3 调用 chat.SetStyleInstructionForSession 实时更新会话风格指令；
// 1.4 若作用范围为群聊，同步持久化到 `onebot_groups.persona` 字段。
func (t *SetPersonaTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 校验管理员权限
	caller, _, err := authorizeCaller(ctx, t.store, RoleAdmin)
	if err != nil {
		return permissionDeniedOutput(err), nil
	}

	var args setPersonaArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return invalidInputOutput("解析参数失败: " + err.Error()), nil
	}
	instruction := strings.TrimSpace(args.StyleInstruction)

	// 1.2 解析目标会话
	sessionID, resolvedScope, resolvedID, err := resolveScopedSession(ctx, t.store, t.chat, caller, args.Scope, args.TargetID, true)
	if err != nil {
		return invalidInputOutput(err.Error()), nil
	}

	// 1.3 调用 ChatService 设置会话级 StyleInstruction
	if sessionID != "" && t.chat != nil {
		if err := t.chat.SetStyleInstructionForSession(ctx, sessionID, instruction); err != nil {
			return executionFailedOutput("set_style_failed", err), nil
		}
	}

	// 1.4 若为群聊作用域，同步写入 onebot_groups.persona 持久化保存；若为私聊作用域，同步写入 onebot_users.persona
	if resolvedScope == "group" && resolvedID > 0 {
		group, found, getErr := t.store.GetGroup(ctx, resolvedID)
		if getErr != nil {
			return executionFailedOutput("db_error", getErr), nil
		}
		if !found {
			group = GroupConfig{
				GroupID:   resolvedID,
				Enabled:   true,
				SessionID: sessionID,
			}
		}
		group.Persona = instruction
		group.UpdatedBy = caller.UserID
		group.UpdatedAt = time.Now().UTC()
		if upErr := t.store.UpsertGroup(ctx, group); upErr != nil {
			return executionFailedOutput("db_error", upErr), nil
		}
	} else if (resolvedScope == "private" || resolvedScope == "current") && resolvedID > 0 {
		user, found, getErr := t.store.GetUser(ctx, resolvedID)
		if getErr == nil && found {
			user.Persona = instruction
			user.UpdatedAt = time.Now().UTC()
			_ = t.store.UpsertUser(ctx, user)
		}
	}

	msg := "已更新机器人说话风格/人设"
	if instruction == "" {
		msg = "已恢复机器人默认说话风格"
	}
	return jsonSuccessOutput(map[string]any{
		"message":           msg,
		"scope":             resolvedScope,
		"target_id":         resolvedID,
		"session_id":        sessionID,
		"style_instruction": instruction,
	})
}

// ============================================================================
// 6.5 onebot_switch_model — 查看可用模型、切换模型及调整推理参数
// ============================================================================

// SwitchModelTool 实现 `onebot_switch_model` 工具，支持模型列表查询、模型切换与采样参数调整。
type SwitchModelTool struct {
	store Store
	chat  ChatFacade
}

type switchModelArgs struct {
	Action      string   `json:"action"`
	ModelID     string   `json:"model_id,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	TargetID    int64    `json:"target_id,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
}

// NewSwitchModelTool 创建 `onebot_switch_model` 工具实例。
func NewSwitchModelTool(store Store, chat ChatFacade) *SwitchModelTool {
	return &SwitchModelTool{store: store, chat: chat}
}

func (t *SwitchModelTool) Name() string {
	return "onebot_switch_model"
}

func (t *SwitchModelTool) Description() string {
	return "查看可用大模型列表、切换指定会话的大模型，或调整推理参数（temperature / max_tokens / top_p）。需要 admin 或 super_admin 权限。"
}

func (t *SwitchModelTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "switch", "set_params", "reset_params"},
				"description": "操作类型：list（列出可用模型）、switch（切换模型，可同时传参数）、set_params（仅调整推理参数）、reset_params（重置推理参数为默认值）",
			},
			"model_id": map[string]any{
				"type":        "string",
				"description": "当 action 为 switch 时的目标模型 ID",
			},
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{"current", "private", "group"},
				"description": "作用范围：current（默认）、private（私聊）、group（群聊）",
			},
			"target_id": map[string]any{
				"type":        "integer",
				"description": "可选的目标 QQ 号或群号",
			},
			"temperature": map[string]any{
				"type":        "number",
				"description": "采样温度，范围 [0.0, 2.0]",
			},
			"max_tokens": map[string]any{
				"type":        "integer",
				"description": "最大输出 Token 数，需大于 0",
			},
			"top_p": map[string]any{
				"type":        "number",
				"description": "核采样阈值 top_p，范围 (0.0, 1.0]",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SwitchModelTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

// Call 执行模型查看、切换或推理参数配置。
// 1.1 校验调用者是否为 admin 或 super_admin；
// 1.2 当 action == "list" 时，返回所有已启用模型的 ID、名称、协议及默认参数；
// 1.3 解析目标会话 SessionID；
// 1.4 根据 action 分支执行模型切换（SwitchModelForSession）或生成参数调整（SetGenerationSettingsForSession）。
func (t *SwitchModelTool) Call(ctx context.Context, rawArgs json.RawMessage) (tooldef.ToolOutput, error) {
	// 1.1 校验管理员权限
	caller, _, err := authorizeCaller(ctx, t.store, RoleAdmin)
	if err != nil {
		return permissionDeniedOutput(err), nil
	}
	if t.chat == nil {
		return executionFailedOutput("service_unavailable", fmt.Errorf("chat service is nil")), nil
	}

	var args switchModelArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return invalidInputOutput("解析参数失败: " + err.Error()), nil
	}
	action := strings.ToLower(strings.TrimSpace(args.Action))

	// 1.2 列出所有可用模型
	if action == "list" {
		models := t.chat.ListModels()
		items := make([]map[string]any, 0, len(models))
		for _, m := range models {
			items = append(items, map[string]any{
				"id":         m.ID,
				"name":       m.Name,
				"provider":   m.Provider,
				"model_name": m.ModelName,
				"enabled":    m.Enabled,
				"is_default": m.IsDefault,
			})
		}
		return jsonSuccessOutput(map[string]any{
			"current_model_id": t.chat.CurrentModelID(),
			"models":           items,
		})
	}

	// 1.3 解析目标会话
	sessionID, resolvedScope, resolvedID, err := resolveScopedSession(ctx, t.store, t.chat, caller, args.Scope, args.TargetID, true)
	if err != nil {
		return invalidInputOutput(err.Error()), nil
	}

	// 1.4 根据具体 action 执行模型切换或参数修改
	switch action {
	case "switch":
		modelID := strings.TrimSpace(args.ModelID)
		if modelID == "" {
			return invalidInputOutput("action 为 switch 时必须提供 model_id"), nil
		}
		if err := t.chat.SwitchModelForSession(ctx, sessionID, modelID); err != nil {
			return executionFailedOutput("switch_model_failed", err), nil
		}
		if args.Temperature != nil || args.MaxTokens != nil || args.TopP != nil {
			if err := t.applyGenerationParams(ctx, sessionID, args); err != nil {
				return invalidInputOutput(err.Error()), nil
			}
		}
		return jsonSuccessOutput(map[string]any{
			"message":    fmt.Sprintf("已将 %s (%d) 的模型切换为 %s", resolvedScope, resolvedID, modelID),
			"scope":      resolvedScope,
			"target_id":  resolvedID,
			"session_id": sessionID,
			"model_id":   modelID,
		})

	case "set_params":
		if args.Temperature == nil && args.MaxTokens == nil && args.TopP == nil {
			return invalidInputOutput("action 为 set_params 时至少需要提供 temperature、max_tokens 或 top_p 之一"), nil
		}
		if err := t.applyGenerationParams(ctx, sessionID, args); err != nil {
			return invalidInputOutput(err.Error()), nil
		}
		return jsonSuccessOutput(map[string]any{
			"message":     fmt.Sprintf("已更新 %s (%d) 的模型推理参数", resolvedScope, resolvedID),
			"scope":       resolvedScope,
			"target_id":   resolvedID,
			"session_id":  sessionID,
			"temperature": args.Temperature,
			"max_tokens":  args.MaxTokens,
			"top_p":       args.TopP,
		})

	case "reset_params":
		if err := t.chat.SetGenerationSettingsForSession(ctx, sessionID, generation.Settings{}); err != nil {
			return executionFailedOutput("reset_params_failed", err), nil
		}
		return jsonSuccessOutput(map[string]any{
			"message":    fmt.Sprintf("已重置 %s (%d) 的模型推理参数为默认值", resolvedScope, resolvedID),
			"scope":      resolvedScope,
			"target_id":  resolvedID,
			"session_id": sessionID,
		})

	default:
		return invalidInputOutput("不支持的 action，仅支持 list / switch / set_params / reset_params"), nil
	}
}

// applyGenerationParams 校验并合并更新指定会话的采样参数。
// 2.1 校验 temperature [0, 2]、top_p (0, 1] 与 max_tokens > 0 的数值范围；
// 2.2 读取当前会话已有的 SessionOverrides 并合并新传入的非空字段；
// 2.3 调用 SetGenerationSettingsForSession 持久化参数。
func (t *SwitchModelTool) applyGenerationParams(ctx context.Context, sessionID string, args switchModelArgs) error {
	if args.Temperature != nil && (*args.Temperature < 0 || *args.Temperature > 2) {
		return fmt.Errorf("temperature 必须在 [0.0, 2.0] 范围内，当前值: %v", *args.Temperature)
	}
	if args.TopP != nil && (*args.TopP <= 0 || *args.TopP > 1) {
		return fmt.Errorf("top_p 必须在 (0.0, 1.0] 范围内，当前值: %v", *args.TopP)
	}
	if args.MaxTokens != nil && *args.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens 必须大于 0，当前值: %d", *args.MaxTokens)
	}

	settings := generation.Settings{}
	if prefs, err := t.chat.SessionPreferencesForSession(ctx, sessionID); err == nil {
		settings = generation.Clone(prefs.SessionOverrides)
	}
	if args.Temperature != nil {
		v := *args.Temperature
		settings.Temperature = &v
	}
	if args.TopP != nil {
		v := *args.TopP
		settings.TopP = &v
	}
	if args.MaxTokens != nil {
		v := *args.MaxTokens
		settings.MaxOutputTokens = &v
	}
	return t.chat.SetGenerationSettingsForSession(ctx, sessionID, settings)
}
