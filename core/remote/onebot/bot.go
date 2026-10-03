package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	mathRand "math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/llm"
	"myai/core/session"
)

// Bot 是 OneBot v11 (NapCatQQ) 正向 WebSocket 客户端与消息路由器。
// 1.1 管理与 NapCatQQ 的长连接、Token 鉴权与指数退避自动重连；
// 1.2 执行五步消息过滤流水线（非消息过滤、黑名单拦截、群开关与 @判定、普通用户静默冷却）；
// 1.3 结合 SessionGuard、双模群会话隔离、Context 查表鉴权与 GroupCompactor 驱动 ChatService。
type Bot struct {
	config    Config
	store     Store
	chat      ChatFacade
	guard     *SessionGuard
	compactor *GroupCompactor

	writeMu   sync.Mutex
	conn      *websocket.Conn
	connected atomic.Bool
	selfID    atomic.Int64
	latencyMS atomic.Int64
	echoSeq   atomic.Uint64
	startedAt time.Time

	cooldownMu    sync.Mutex
	lastTriggered map[int64]time.Time

	perUserGroupMu       sync.Mutex
	perUserGroupSessions map[string]string
}

// NewBot 创建并初始化一个 OneBot 机器人实例。
// 1.1 规范化启动配置 Config；
// 1.2 若 store 为 nil，自动回退为内存存储 NewMemoryStore()；
// 1.3 初始化 SessionGuard 并发保护器与 GroupCompactor 三段式会话压缩器。
func NewBot(config Config, store Store, chat ChatFacade) (*Bot, error) {
	normalized, err := config.Normalize()
	if err != nil {
		return nil, err
	}
	if store == nil {
		store = NewMemoryStore()
	}
	return &Bot{
		config:               normalized,
		store:                store,
		chat:                 chat,
		guard:                NewSessionGuard(),
		compactor:            NewGroupCompactor(chat),
		startedAt:            time.Now().UTC(),
		lastTriggered:        make(map[int64]time.Time),
		perUserGroupSessions: make(map[string]string),
	}, nil
}

// Run 启动正向 WebSocket 客户端主循环，并在断线时按指数退避策略（1s ~ 30s）自动重连。
// 2.1 启动前先将命令行参数中的 --super-admin 列表种子化写入 `onebot_users` 表；
// 2.2 循环发起 WebSocket 连接，若 ctx 被取消则优雅退出；
// 2.3 若连接意外断开，计算带随机抖动的指数退避等待时间后重连。
func (b *Bot) Run(ctx context.Context) error {
	if b.chat == nil {
		return errors.New("chat service is nil")
	}

	// 2.1 种子化超级管理员 QQ 号列表
	if len(b.config.SuperAdmins) > 0 {
		if err := b.store.SeedSuperAdmins(ctx, b.config.SuperAdmins); err != nil {
			return fmt.Errorf("seed super admins failed: %w", err)
		}
	}

	log.Printf("[onebot] starting bot, connecting to %s (group_at_only=%v, group_session_per_user=%v)",
		b.config.WSURL, b.config.GroupAtOnly, b.config.GroupSessionPerUser)

	// 2.2 进入正向 WebSocket 自动重连主循环
	backoff := time.Second
	for {
		connectedAt := time.Now()
		wasConnected, err := b.runSingleConnection(ctx)
		if ctx.Err() != nil {
			log.Printf("[onebot] bot stopped.")
			return nil
		}
		if err != nil {
			log.Printf("[onebot] connection closed: %v; reconnecting in %s", err, backoff)
		}

		// 2.3 若上次连接稳定持续超过 30 秒，则重置退避时间为 1 秒
		if wasConnected && time.Since(connectedAt) >= 30*time.Second {
			backoff = time.Second
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Printf("[onebot] bot stopped.")
			return nil
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			jitter := 0.8 + 0.4*mathRand.Float64()
			backoff = time.Duration(float64(backoff) * 1.5 * jitter)
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

// runSingleConnection 维护单次 WebSocket 连接的完整生命周期。
// 3.1 构造携带 `Authorization: Bearer <Token>` 的握手 Header 并拨号连接 NapCatQQ；
// 3.2 注册 Ping/Pong 处理器以测量通信延迟；
// 3.3 循环读取并分发 OneBot v11 事件数据包。
func (b *Bot) runSingleConnection(ctx context.Context) (bool, error) {
	// 3.1 构造鉴权 Header 并发起正向 WebSocket 连接
	headers := http.Header{}
	if b.config.Token != "" {
		headers.Set("Authorization", "Bearer "+b.config.Token)
	}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, b.config.WSURL, headers)
	if err != nil {
		if resp != nil {
			return false, fmt.Errorf("dial %s failed (status %s): %w", b.config.WSURL, resp.Status, err)
		}
		return false, fmt.Errorf("dial %s failed: %w", b.config.WSURL, err)
	}

	b.setConnection(conn)
	defer func() {
		b.clearConnection(conn)
		_ = conn.Close()
	}()

	log.Printf("[onebot] connected to NapCatQQ at %s", b.config.WSURL)

	// 3.2 监听 Context 取消信号并循环读取 WebSocket 帧（将 JSON 解析错误与底层断线解耦，防止异常包导致断连）
	readDone := make(chan error, 1)
	go func() {
		for {
			_, rawPacket, err := conn.ReadMessage()
			if err != nil {
				readDone <- err
				return
			}
			var event Event
			if err := json.Unmarshal(rawPacket, &event); err != nil {
				log.Printf("[onebot] ignore unrecognized packet: %v", err)
				continue
			}
			b.dispatchRawEvent(ctx, event)
		}
	}()

	select {
	case <-ctx.Done():
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "onebot shutting down"))
		return true, nil
	case err := <-readDone:
		return true, err
	}
}

// dispatchRawEvent 处理从 WebSocket 收到的每一个 OneBot v11 数据包。
// 4.1 若事件包含 SelfID，则更新机器人自身 QQ 号记录；
// 4.2 若为 meta_event（心跳或生命周期事件），计算与事件时间戳的延迟并忽略后续处理；
// 4.3 若为 message 事件，异步交由 HandleEvent 执行过滤与对话处理。
func (b *Bot) dispatchRawEvent(ctx context.Context, event Event) {
	if event.SelfID > 0 {
		b.selfID.Store(event.SelfID)
	}
	if event.Time > 0 {
		latency := time.Now().UnixMilli() - event.Time*1000
		if latency < 0 {
			latency = 0
		}
		b.latencyMS.Store(latency)
	}

	if event.PostType == "meta_event" {
		return
	}
	if event.PostType == "message" {
		go b.HandleEvent(ctx, event)
	}
}

// HandleEvent 执行完整的五步消息过滤、身份核验、并发调度与对话推理流程（同时暴露供单元测试直接调用）。
// 5.1 基础过滤：忽略非 message 事件、无效发送者或机器人自身发出的消息；
// 5.2 消息段解析：提取纯文本、@机器人 标记以及 @其他成员 的 QQ 号；
// 5.3 用户身份查表与自动登记：查询 `onebot_users`，若为 banned 黑名单用户则直接静默丢弃；
// 5.4 群聊规则过滤：若为群聊，校验是否满足 `@机器人` 条件以及 `onebot_groups.enabled` 开关（管理员不受关闭限制以便重新开启）；
// 5.5 普通用户冷却拦截：若 role == user 且距离上次触发不足 Cooldown，静默丢弃不做任何回复；
// 5.6 交由 SessionGuard 进行同会话并发保护与排队执行。
func (b *Bot) HandleEvent(ctx context.Context, event Event) {
	// 5.1 基础过滤：仅处理来自真实用户的 message 事件
	if event.PostType != "message" || event.UserID <= 0 {
		return
	}
	selfID := event.SelfID
	if selfID <= 0 {
		selfID = b.selfID.Load()
	}
	if selfID > 0 && event.UserID == selfID {
		return
	}

	// 5.2 解析 OneBot v11 数组消息段
	segments, err := event.ParseSegments()
	if err != nil {
		log.Printf("[onebot] parse segments failed: %v", err)
		return
	}
	parsed := ExtractMessageContent(segments, selfID)
	if parsed.CleanText == "" {
		return
	}

	// 5.3 查询或自动登记发送者身份，拦截黑名单用户
	now := time.Now().UTC()
	nickname := event.Sender.DisplayName(event.UserID)
	user, found, err := b.store.GetUser(ctx, event.UserID)
	if err != nil {
		log.Printf("[onebot] get user %d failed: %v", event.UserID, err)
		return
	}
	if !found {
		user = UserIdentity{
			UserID:       event.UserID,
			Nickname:     nickname,
			Role:         RoleUser,
			LastActiveAt: now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		_ = b.store.UpsertUser(ctx, user)
	}
	if user.Role == RoleBanned {
		// 黑名单用户直接静默丢弃，不消耗任何大模型 Token
		return
	}

	// 5.4 群聊场景过滤：检查 @机器人 与群开关状态
	if event.MessageType == "group" {
		if event.GroupID <= 0 {
			return
		}
		if b.config.GroupAtOnly && !parsed.MentionedBot {
			return
		}
		group, groupFound, groupErr := b.store.GetGroup(ctx, event.GroupID)
		if groupErr != nil {
			log.Printf("[onebot] get group %d failed: %v", event.GroupID, groupErr)
			return
		}
		if !groupFound {
			group = GroupConfig{
				GroupID:   event.GroupID,
				Enabled:   true,
				UpdatedAt: now,
			}
			_ = b.store.UpsertGroup(ctx, group)
		}
		// 若群服务已关闭，仅允许 admin 或 super_admin 继续交互（以便通过自然语言重新开启本群）
		if !group.Enabled && !HasAtLeastRole(user.Role, RoleAdmin) {
			return
		}
	}

	// 5.5 普通用户冷却检查（静默丢弃策略）
	if user.Role == RoleUser && b.config.Cooldown > 0 && !IsCancelCommand(parsed.CleanText) {
		if !b.allowCooldown(event.UserID, now) {
			return
		}
	}

	// 更新用户最后活跃时间与最新昵称
	user.Nickname = nickname
	user.LastActiveAt = now
	user.UpdatedAt = now
	_ = b.store.UpsertUser(ctx, user)

	log.Printf("[onebot] recv %s message from %s(%d, role=%s): %s",
		event.MessageType, nickname, event.UserID, user.Role, parsed.CleanText)

	// 5.6 计算并发锁键并交由 SessionGuard 调度
	guardKey := b.sessionGuardKey(event)
	guardResult := b.guard.Submit(ctx, guardKey, parsed.CleanText, func(runCtx context.Context) {
		b.executeTurn(runCtx, event, parsed.CleanText)
	})

	switch guardResult {
	case GuardResultCancelled:
		_ = b.replyToEvent(ctx, event, "已停止当前回答任务。")
	case GuardResultNoActiveTask:
		_ = b.replyToEvent(ctx, event, "当前没有正在运行的回答任务。")
	}
}

// executeTurn 在 SessionGuard 保护下执行单轮完整的大模型推理、工具调用与回复发送。
// 6.1 实时重新读取发送者角色（确保排队期间若被封禁或提权能立即生效）；
// 6.2 为私聊或群聊分配/获取绑定的 MyAI SessionID；
// 6.3 按发送者角色与会话场景同步设置该 Session 的 PermissionMode（超级管理员私聊为 Full，其余为 Readonly）；
// 6.4 将不可伪造的 CallerInfo 注入 Context，并在用户输入头部拼接结构化身份前缀；
// 6.5 调用 ChatService.SendMessageStreamForSession 获取回答，触发水位压缩检查，并发送回 NapCatQQ。
func (b *Bot) executeTurn(ctx context.Context, event Event, cleanText string) {
	// 6.1 重新核验用户角色
	user, found, err := b.store.GetUser(ctx, event.UserID)
	if err != nil {
		log.Printf("[onebot] reload user %d failed: %v", event.UserID, err)
		return
	}
	if !found {
		user = UserIdentity{
			UserID:   event.UserID,
			Nickname: event.Sender.DisplayName(event.UserID),
			Role:     RoleUser,
		}
	}
	if user.Role == RoleBanned {
		return
	}

	// 6.2 获取或创建绑定的 MyAI SessionID
	sessionID, err := b.ensureSessionForEvent(ctx, event, &user)
	if err != nil {
		log.Printf("[onebot] ensure session failed: %v", err)
		_ = b.replyToEvent(ctx, event, "初始化会话失败，请稍后重试。")
		return
	}

	// 6.3 动态设置会话的底层工具权限模式
	permMode := string(session.PermissionModeReadonly)
	if event.MessageType == "private" && user.Role == RoleSuperAdmin {
		permMode = string(session.PermissionModeFull)
	}
	_ = b.chat.SetPermissionModeForSession(ctx, sessionID, permMode)

	// 6.4 注入真实 CallerInfo 到 Context 并构造带身份头部的 Prompt
	caller := CallerInfo{
		UserID:      event.UserID,
		Nickname:    event.Sender.DisplayName(event.UserID),
		GroupID:     event.GroupID,
		MessageType: event.MessageType,
		MessageID:   event.MessageID,
		SessionID:   sessionID,
	}
	callCtx := WithCallerInfo(ctx, caller)
	promptInput := FormatPromptHeader(caller, user.Role) + "\n" + cleanText

	// 6.5 调用 ChatService 生成回复
	resp, err := b.chat.SendMessageStreamForSession(callCtx, sessionID, promptInput, llm.ChatStreamHandler{})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(callCtx.Err(), context.Canceled) {
			return
		}
		log.Printf("[onebot] chat generation failed (session=%s): %v", sessionID, err)
		_ = b.replyToEvent(ctx, event, "抱歉，处理您的请求时遇到了错误："+err.Error())
		return
	}

	// 检查上下文水位并按需执行软/硬阈值压缩
	if b.compactor != nil {
		b.compactor.CheckAndCompact(callCtx, sessionID, resp)
	}

	replyText := strings.TrimSpace(resp.Result.Content)
	if replyText == "" {
		replyText = "（已完成操作）"
	}
	if err := b.replyToEvent(ctx, event, replyText); err != nil {
		log.Printf("[onebot] send reply failed: %v", err)
		return
	}
	log.Printf("[onebot] sent reply to %s(%d) (session=%s)", caller.Nickname, event.UserID, sessionID)
}

// ensureSessionForEvent 根据私聊或群聊（共享模式 vs 群内独立模式）解析或创建对应的 MyAI SessionID。
// 7.1 私聊模式：使用 `onebot_users.private_session_id`，为空或已失效时创建新会话并回写；
// 7.2 群聊独立模式（GroupSessionPerUser == true）：按 `group:<groupID>:user:<userID>` 维护独立会话；
// 7.3 群聊共享模式（默认）：使用 `onebot_groups.session_id`，创建时自动应用 `group.Persona` 风格指令。
func (b *Bot) ensureSessionForEvent(ctx context.Context, event Event, user *UserIdentity) (string, error) {
	if event.MessageType == "group" && event.GroupID > 0 {
		// 7.2 群内每人独立 Session 模式
		if b.config.GroupSessionPerUser {
			key := fmt.Sprintf("group:%d:user:%d", event.GroupID, event.UserID)
			b.perUserGroupMu.Lock()
			sid := b.perUserGroupSessions[key]
			b.perUserGroupMu.Unlock()

			if sid != "" && b.sessionExists(ctx, sid) {
				return sid, nil
			}
			newID, err := b.chat.CreateSession(ctx, fmt.Sprintf("QQ群%d-用户%d", event.GroupID, event.UserID))
			if err != nil {
				return "", err
			}
			if group, ok, _ := b.store.GetGroup(ctx, event.GroupID); ok && strings.TrimSpace(group.Persona) != "" {
				_ = b.chat.SetStyleInstructionForSession(ctx, newID, group.Persona)
			}
			b.perUserGroupMu.Lock()
			b.perUserGroupSessions[key] = newID
			b.perUserGroupMu.Unlock()
			return newID, nil
		}

		// 7.3 群聊共享 Session 模式（默认）
		group, found, err := b.store.GetGroup(ctx, event.GroupID)
		if err != nil {
			return "", err
		}
		if !found {
			group = GroupConfig{
				GroupID: event.GroupID,
				Enabled: true,
			}
		}
		if group.SessionID != "" && b.sessionExists(ctx, group.SessionID) {
			return group.SessionID, nil
		}
		newID, err := b.chat.CreateSession(ctx, fmt.Sprintf("QQ群聊:%d", event.GroupID))
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(group.Persona) != "" {
			_ = b.chat.SetStyleInstructionForSession(ctx, newID, group.Persona)
		}
		group.SessionID = newID
		group.UpdatedAt = time.Now().UTC()
		if err := b.store.UpsertGroup(ctx, group); err != nil {
			return "", err
		}
		return newID, nil
	}

	// 7.1 私聊模式
	if user.PrivateSessionID != "" && b.sessionExists(ctx, user.PrivateSessionID) {
		return user.PrivateSessionID, nil
	}
	newID, err := b.chat.CreateSession(ctx, fmt.Sprintf("QQ私聊:%d", event.UserID))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(user.Persona) != "" {
		_ = b.chat.SetStyleInstructionForSession(ctx, newID, user.Persona)
	}
	user.PrivateSessionID = newID
	user.UpdatedAt = time.Now().UTC()
	if err := b.store.UpsertUser(ctx, *user); err != nil {
		return "", err
	}
	return newID, nil
}

// sessionExists 检查指定 sessionID 在 ChatService 中是否仍然有效。
func (b *Bot) sessionExists(ctx context.Context, sessionID string) bool {
	if strings.TrimSpace(sessionID) == "" || b.chat == nil {
		return false
	}
	_, err := b.chat.ContextInfoForSession(ctx, sessionID)
	return err == nil
}

// sessionGuardKey 计算当前事件对应的并发锁粒度键。
func (b *Bot) sessionGuardKey(event Event) string {
	if event.MessageType == "group" && event.GroupID > 0 {
		if b.config.GroupSessionPerUser {
			return fmt.Sprintf("group:%d:user:%d", event.GroupID, event.UserID)
		}
		return fmt.Sprintf("group:%d", event.GroupID)
	}
	return fmt.Sprintf("private:%d", event.UserID)
}

// allowCooldown 判断普通用户是否已度过冷却期，若允许则更新触发时间戳。
func (b *Bot) allowCooldown(userID int64, now time.Time) bool {
	b.cooldownMu.Lock()
	defer b.cooldownMu.Unlock()

	if last, ok := b.lastTriggered[userID]; ok {
		if now.Sub(last) < b.config.Cooldown {
			return false
		}
	}
	b.lastTriggered[userID] = now
	return true
}

// replyToEvent 根据原事件类型（群聊带引用回复 vs 私聊直接回复）发送响应消息。
func (b *Bot) replyToEvent(ctx context.Context, event Event, text string) error {
	echo := fmt.Sprintf("reply-%d", b.echoSeq.Add(1))
	if event.MessageType == "group" && event.GroupID > 0 {
		action := BuildGroupMsgAction(event.GroupID, event.MessageID, text, echo)
		return b.sendAction(ctx, action)
	}
	action := BuildPrivateMsgAction(event.UserID, text, echo)
	return b.sendAction(ctx, action)
}

// SendTextMessage 实现 MessageSender 接口，供 `onebot_send_message` 工具主动向指定私聊或群聊发送文本消息。
// 8.1 根据 messageType 构造私聊或群聊 ActionRequest；
// 8.2 调用 sendAction 通过当前活跃的 WebSocket 连接发送。
func (b *Bot) SendTextMessage(ctx context.Context, messageType string, targetID int64, content string) error {
	echo := fmt.Sprintf("tool-send-%d", b.echoSeq.Add(1))
	if strings.EqualFold(strings.TrimSpace(messageType), "group") {
		return b.sendAction(ctx, BuildGroupMsgAction(targetID, 0, content, echo))
	}
	return b.sendAction(ctx, BuildPrivateMsgAction(targetID, content, echo))
}

// RuntimeStatus 实现 MessageSender 接口，返回当前 WebSocket 连接与进程运行状态。
func (b *Bot) RuntimeStatus() BotRuntimeStatus {
	uptime := int64(time.Since(b.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}
	return BotRuntimeStatus{
		Connected:              b.connected.Load(),
		WSURL:                  b.config.WSURL,
		SelfID:                 b.selfID.Load(),
		LastHeartbeatLatencyMS: b.latencyMS.Load(),
		StartedAt:              b.startedAt,
		UptimeSeconds:          uptime,
	}
}

// sendAction 线程安全地向当前活跃的 WebSocket 连接写入 JSON 动作请求。
func (b *Bot) sendAction(_ context.Context, action ActionRequest) error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	if b.conn == nil || !b.connected.Load() {
		return errors.New("NapCatQQ WebSocket 当前未连接")
	}
	_ = b.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return b.conn.WriteJSON(action)
}

func (b *Bot) setConnection(conn *websocket.Conn) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	b.conn = conn
	b.connected.Store(conn != nil)
}

func (b *Bot) clearConnection(conn *websocket.Conn) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	if b.conn == conn {
		b.conn = nil
		b.connected.Store(false)
	}
}
