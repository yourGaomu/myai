package onebot

import (
	"context"
	"fmt"
	"strings"
)

// Role 定义 OneBot 用户的四级权限角色枚举。
// 1.1 RoleSuperAdmin：超级管理员（最高权限，可任命/撤销 admin，私聊拥有 Full 权限）；
// 1.2 RoleAdmin：普通管理员（可管理群开关、人设、模型、清空历史、封禁普通用户）；
// 1.3 RoleUser：普通用户（仅可与机器人正常对话，受冷却时间与 Readonly 权限限制）；
// 1.4 RoleBanned：黑名单封禁用户（消息在入口处直接静默丢弃，不调用大模型）。
type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleAdmin      Role = "admin"
	RoleUser       Role = "user"
	RoleBanned     Role = "banned"
)

// NormalizeRole 将任意角色字符串规范化为标准 Role 枚举。
// 1.1 清理首尾空格并转为小写；
// 1.2 匹配已知的四种合法角色，未知角色安全回退为普通用户 RoleUser。
func NormalizeRole(raw string) Role {
	switch Role(strings.ToLower(strings.TrimSpace(raw))) {
	case RoleSuperAdmin:
		return RoleSuperAdmin
	case RoleAdmin:
		return RoleAdmin
	case RoleBanned:
		return RoleBanned
	default:
		return RoleUser
	}
}

// IsValidRole 校验给定角色字符串是否为受支持的合法角色。
// 1.1 判断是否属于 super_admin / admin / user / banned 之一。
func IsValidRole(raw string) bool {
	switch Role(strings.ToLower(strings.TrimSpace(raw))) {
	case RoleSuperAdmin, RoleAdmin, RoleUser, RoleBanned:
		return true
	default:
		return false
	}
}

// RoleLevel 返回角色的数值等级以便进行权限高低比较。
// 1.1 super_admin = 3；
// 1.2 admin = 2；
// 1.3 user = 1；
// 1.4 banned = 0。
func RoleLevel(r Role) int {
	switch NormalizeRole(string(r)) {
	case RoleSuperAdmin:
		return 3
	case RoleAdmin:
		return 2
	case RoleUser:
		return 1
	case RoleBanned:
		return 0
	default:
		return 1
	}
}

// HasAtLeastRole 判断当前角色 actual 是否满足最低要求角色 required。
// 1.1 比较两个角色的数值等级 RoleLevel(actual) >= RoleLevel(required)。
func HasAtLeastRole(actual Role, required Role) bool {
	return RoleLevel(actual) >= RoleLevel(required)
}

// CallerInfo 保存从 NapCatQQ 底层 WebSocket 事件提取的真实调用者身份信息。
// 2.1 该结构通过 Go context.Context 向下传递（类似 Spring Security 的 SecurityContextHolder）；
// 2.2 任何管理类 Tool 在执行时必须从 Context 读取 CallerInfo，绝不信任用户在 Prompt 中自称的身份。
type CallerInfo struct {
	UserID      int64  `json:"user_id"`      // 真实发送者 QQ 号（来自底层事件，不可伪造）
	Nickname    string `json:"nickname"`     // 发送者 QQ 昵称或群名片
	GroupID     int64  `json:"group_id"`     // 所在群号（私聊时为 0）
	MessageType string `json:"message_type"` // "private" 或 "group"
	MessageID   int32  `json:"message_id"`   // 原消息 ID，用于群聊引用回复
	SessionID   string `json:"session_id"`   // 当前对话绑定的 MyAI SessionID
}

// callerContextKey 是用于在 context.Context 中存取 CallerInfo 的私有键类型，防止跨包键冲突。
type callerContextKey struct{}

// WithCallerInfo 将真实发送者信息注入到 context.Context 中。
// 3.1 若父级 ctx 为 nil，则自动初始化为 context.Background()；
// 3.2 使用私有键类型 callerContextKey 写入 CallerInfo。
func WithCallerInfo(ctx context.Context, info CallerInfo) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, callerContextKey{}, info)
}

// CallerFromContext 从 context.Context 中提取真实发送者信息。
// 3.1 若 ctx 为 nil 或未包含 CallerInfo，则返回零值与 false；
// 3.2 提取成功则返回对应的 CallerInfo 与 true。
func CallerFromContext(ctx context.Context) (CallerInfo, bool) {
	if ctx == nil {
		return CallerInfo{}, false
	}
	info, ok := ctx.Value(callerContextKey{}).(CallerInfo)
	return info, ok
}

// FormatPromptHeader 构造注入到每条用户消息头部的结构化身份元信息前缀。
// 4.1 区分群聊与私聊场景，生成包含会话类型、群号、发送者 QQ、昵称与数据库角色的安全前缀；
// 4.2 帮助大模型在群聊共享 Session 中准确区分不同发言人，同时理解当前发言人的权限等级。
func FormatPromptHeader(caller CallerInfo, role Role) string {
	scene := "私聊"
	groupPart := ""
	if caller.MessageType == "group" && caller.GroupID > 0 {
		scene = "群聊"
		groupPart = fmt.Sprintf(" | 群号: %d", caller.GroupID)
	}
	nick := strings.TrimSpace(caller.Nickname)
	if nick == "" {
		nick = fmt.Sprintf("QQ_%d", caller.UserID)
	}
	return fmt.Sprintf("[当前会话: %s%s | 发送者QQ: %d | 昵称: %s | 身份: %s]",
		scene, groupPart, caller.UserID, nick, NormalizeRole(string(role)))
}
