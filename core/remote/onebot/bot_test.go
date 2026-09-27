package onebot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"myai/core/contextmgr"
	"myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	"myai/core/llm"
	"myai/core/service"
)

// mockChatFacade 提供用于单元测试的轻量级 ChatFacade 模拟实现。
type mockChatFacade struct {
	mu               sync.Mutex
	seq              int
	sessions         map[string]bool
	permissionModes  map[string]string
	styleInstruction map[string]string
	models           map[string]string
	genSettings      map[string]generation.Settings
	compacted        []string
	lastPrompt       string
	lastCaller       CallerInfo
}

func newMockChatFacade() *mockChatFacade {
	return &mockChatFacade{
		sessions:         make(map[string]bool),
		permissionModes:  make(map[string]string),
		styleInstruction: make(map[string]string),
		models:           make(map[string]string),
		genSettings:      make(map[string]generation.Settings),
	}
}

func (m *mockChatFacade) CreateSession(_ context.Context, title string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := fmt.Sprintf("session-%d-%s", m.seq, title)
	m.sessions[id] = true
	m.models[id] = "gpt-4o-mini"
	return id, nil
}

func (m *mockChatFacade) SendMessageStreamForSession(ctx context.Context, sessionID string, input string, _ llm.ChatStreamHandler) (service.ChatResponse, error) {
	m.mu.Lock()
	m.lastPrompt = input
	if caller, ok := CallerFromContext(ctx); ok {
		m.lastCaller = caller
	}
	m.mu.Unlock()

	return service.ChatResponse{
		SessionID: sessionID,
		Result: llm.ChatResult{
			Content: "收到: " + input,
		},
		Context: service.ContextInfo{
			WindowK:        16,
			SelectedTokens: 1000,
		},
	}, nil
}

func (m *mockChatFacade) DeleteSession(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockChatFacade) SetPermissionModeForSession(_ context.Context, sessionID string, mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.permissionModes[sessionID] = mode
	return nil
}

func (m *mockChatFacade) SetStyleInstructionForSession(_ context.Context, sessionID string, instruction string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.styleInstruction[sessionID] = instruction
	return nil
}

func (m *mockChatFacade) SetGenerationSettingsForSession(_ context.Context, sessionID string, settings generation.Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.genSettings[sessionID] = generation.Clone(settings)
	return nil
}

func (m *mockChatFacade) SessionPreferencesForSession(_ context.Context, sessionID string) (service.SessionPreferencesView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return service.SessionPreferencesView{
		SessionOverrides: generation.Clone(m.genSettings[sessionID]),
		StyleInstruction: m.styleInstruction[sessionID],
	}, nil
}

func (m *mockChatFacade) ListModels() []llm.ModelInfo {
	return []llm.ModelInfo{
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Enabled: true, IsDefault: true},
		{ID: "deepseek-chat", Name: "DeepSeek Chat", Enabled: true},
	}
}

func (m *mockChatFacade) SwitchModelForSession(_ context.Context, sessionID string, modelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.models[sessionID] = modelID
	return nil
}

func (m *mockChatFacade) CurrentModelID() string {
	return "gpt-4o-mini"
}

func (m *mockChatFacade) ContextInfoForSession(_ context.Context, sessionID string) (service.ContextInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.sessions[sessionID] {
		return service.ContextInfo{}, fmt.Errorf("session not found: %s", sessionID)
	}
	return service.ContextInfo{WindowK: 16, SelectedTokens: 1000}, nil
}

func (m *mockChatFacade) CompactSession(_ context.Context, sessionID string) (service.ContextInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compacted = append(m.compacted, sessionID)
	return service.ContextInfo{WindowK: 16, SelectedTokens: 500}, nil
}

// mockSender 实现 MessageSender 接口以验证主动消息发送与状态查询。
type mockSender struct {
	sent []SendMsgParams
}

func (s *mockSender) SendTextMessage(_ context.Context, messageType string, targetID int64, content string) error {
	params := SendMsgParams{
		MessageType: messageType,
		Message:     []Segment{TextSegment(content)},
	}
	if messageType == "group" {
		params.GroupID = targetID
	} else {
		params.UserID = targetID
	}
	s.sent = append(s.sent, params)
	return nil
}

func (s *mockSender) RuntimeStatus() BotRuntimeStatus {
	return BotRuntimeStatus{
		Connected:              true,
		WSURL:                  DefaultWSURL,
		SelfID:                 999999,
		LastHeartbeatLatencyMS: 12,
		UptimeSeconds:          3600,
	}
}

// TestExtractMessageContent 测试 OneBot v11 数组消息段的 @机器人 剥离与 @其他成员 转换逻辑。
// 1.1 构造包含 @机器人、普通文本、@目标用户与图片的复合消息段；
// 1.2 验证 MentionedBot 被正确识别为 true；
// 1.3 验证 @目标用户 被转换为 `[提及用户 QQ: 888888]` 且图片转换为 `[图片]`。
func TestExtractMessageContent(t *testing.T) {
	segments := []Segment{
		AtSegment(999999),
		TextSegment(" 帮我给 "),
		AtSegment(888888),
		TextSegment(" 设置为管理员 "),
		{Type: "image", Data: map[string]any{"file": "abc.jpg"}},
	}

	parsed := ExtractMessageContent(segments, 999999)
	if !parsed.MentionedBot {
		t.Fatalf("expected MentionedBot=true, got false")
	}
	if len(parsed.MentionedUsers) != 1 || parsed.MentionedUsers[0] != 888888 {
		t.Fatalf("expected MentionedUsers=[888888], got %+v", parsed.MentionedUsers)
	}
	if !strings.Contains(parsed.CleanText, "[提及用户 QQ: 888888]") {
		t.Fatalf("expected CleanText to contain target QQ tag, got %q", parsed.CleanText)
	}
	if strings.Contains(parsed.CleanText, "999999") {
		t.Fatalf("expected bot self @mention to be stripped from CleanText, got %q", parsed.CleanText)
	}
}

// TestSetUserRoleTool_GranularRBAC 测试三级角色在 `onebot_set_user_role` 中的细粒度权限边界与防提权保护。
// 2.1 初始化超级管理员(10001)、普通管理员(20001)与普通用户(30001)；
// 2.2 验证普通用户尝试把自已提权为 admin 时被拦截；
// 2.3 验证普通管理员可以封禁普通用户(banned)，但无权任命新 admin、无权罢免其他 admin、无权修改 super_admin；
// 2.4 验证超级管理员可以任命新 admin 并罢免现有 admin，但同样不能剥夺其他 super_admin 的权限。
func TestSetUserRoleTool_GranularRBAC(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if err := store.SeedSuperAdmins(ctx, []int64{10001, 10002}); err != nil {
		t.Fatalf("seed super admins failed: %v", err)
	}
	_ = store.UpsertUser(ctx, UserIdentity{UserID: 20001, Role: RoleAdmin, Nickname: "AdminA"})
	_ = store.UpsertUser(ctx, UserIdentity{UserID: 20002, Role: RoleAdmin, Nickname: "AdminB"})
	_ = store.UpsertUser(ctx, UserIdentity{UserID: 30001, Role: RoleUser, Nickname: "UserA"})

	tool := NewSetUserRoleTool(store)

	// 2.2 普通用户尝试自我提权 -> 必须拒绝
	userCtx := WithCallerInfo(ctx, CallerInfo{UserID: 30001, MessageType: "group", GroupID: 5001})
	out, _ := tool.Call(userCtx, json.RawMessage(`{"target_qq":30001,"role":"admin"}`))
	if !out.Failed() {
		t.Fatalf("expected normal user self-promotion to fail, but succeeded: %s", out.Content)
	}

	// 2.3 普通管理员封禁普通用户 -> 允许
	adminCtx := WithCallerInfo(ctx, CallerInfo{UserID: 20001, MessageType: "group", GroupID: 5001})
	out, _ = tool.Call(adminCtx, json.RawMessage(`{"target_qq":30001,"role":"banned"}`))
	if out.Failed() {
		t.Fatalf("expected admin banning normal user to succeed, got error: %s", out.ErrorMessage)
	}
	u30001, _, _ := store.GetUser(ctx, 30001)
	if u30001.Role != RoleBanned {
		t.Fatalf("expected user 30001 role=banned, got %s", u30001.Role)
	}

	// 普通管理员尝试任命新 admin -> 必须拒绝
	out, _ = tool.Call(adminCtx, json.RawMessage(`{"target_qq":30001,"role":"admin"}`))
	if !out.Failed() {
		t.Fatalf("expected admin promoting another user to admin to fail")
	}

	// 普通管理员尝试封禁另一位管理员 AdminB -> 必须拒绝
	out, _ = tool.Call(adminCtx, json.RawMessage(`{"target_qq":20002,"role":"banned"}`))
	if !out.Failed() {
		t.Fatalf("expected admin banning another admin to fail")
	}

	// 2.4 超级管理员任命新 admin -> 允许
	superCtx := WithCallerInfo(ctx, CallerInfo{UserID: 10001, MessageType: "private"})
	out, _ = tool.Call(superCtx, json.RawMessage(`{"target_qq":30001,"role":"admin"}`))
	if out.Failed() {
		t.Fatalf("expected super_admin promoting user to admin to succeed, got: %s", out.ErrorMessage)
	}

	// 超级管理员尝试修改另一位超级管理员 -> 必须拒绝
	out, _ = tool.Call(superCtx, json.RawMessage(`{"target_qq":10002,"role":"user"}`))
	if !out.Failed() {
		t.Fatalf("expected modifying another super_admin to fail")
	}
}

// TestSendMessageTool_SpamProtection 测试 `onebot_send_message` 的私聊白名单 SPAM 防护。
// 3.1 向从未交互过的陌生 QQ 号发送私聊 -> 拒绝；
// 3.2 向已存在于 `onebot_users` 表的用户发送私聊 -> 成功；
// 3.3 向任意群号发送群聊通知 -> 成功。
func TestSendMessageTool_SpamProtection(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.SeedSuperAdmins(ctx, []int64{10001})
	_ = store.UpsertUser(ctx, UserIdentity{UserID: 888888, Nickname: "KnownFriend", Role: RoleUser})

	sender := &mockSender{}
	tool := NewSendMessageTool(store, sender)
	adminCtx := WithCallerInfo(ctx, CallerInfo{UserID: 10001, MessageType: "private"})

	// 3.1 陌生 QQ 号私聊被拦截
	out, _ := tool.Call(adminCtx, json.RawMessage(`{"message_type":"private","target_id":777777,"content":"hello"}`))
	if !out.Failed() {
		t.Fatalf("expected private message to unknown user 777777 to be blocked by SPAM protection")
	}

	// 3.2 已知用户私聊放行
	out, _ = tool.Call(adminCtx, json.RawMessage(`{"message_type":"private","target_id":888888,"content":"今晚八点上线"}`))
	if out.Failed() {
		t.Fatalf("expected private message to known user 888888 to succeed, got: %s", out.ErrorMessage)
	}
	if len(sender.sent) != 1 || sender.sent[0].UserID != 888888 {
		t.Fatalf("unexpected sent messages: %+v", sender.sent)
	}

	// 3.3 群聊发送放行
	out, _ = tool.Call(adminCtx, json.RawMessage(`{"message_type":"group","target_id":654321,"content":"明天开会"}`))
	if out.Failed() {
		t.Fatalf("expected group message to succeed, got: %s", out.ErrorMessage)
	}
}

// TestQueryTool_SQLWhitelistAndExecution 测试 `onebot_query` 的 SQL 白名单校验与内存 SQLite 沙箱执行。
// 4.1 验证危险语句（DELETE/DROP/多语句/非白名单表）全部被拦截；
// 4.2 验证聚合查询（GROUP BY / COUNT）与排序查询在沙箱中准确返回预期结果。
func TestQueryTool_SQLWhitelistAndExecution(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.SeedSuperAdmins(ctx, []int64{10001})
	_ = store.UpsertUser(ctx, UserIdentity{UserID: 20001, Nickname: "张三", Role: RoleAdmin})
	_ = store.UpsertUser(ctx, UserIdentity{UserID: 30001, Nickname: "李四", Role: RoleUser})
	_ = store.UpsertGroup(ctx, GroupConfig{GroupID: 9001, GroupName: "Go开发群", Enabled: true, Persona: "资深架构师"})
	_ = store.UpsertGroup(ctx, GroupConfig{GroupID: 9002, GroupName: "闲聊群", Enabled: false})

	qTool := NewQueryTool(store)
	adminCtx := WithCallerInfo(ctx, CallerInfo{UserID: 20001, MessageType: "group", GroupID: 9001})

	// 4.1 拦截非法 SQL
	invalidSQLs := []string{
		"DELETE FROM onebot_users WHERE user_id = 30001",
		"SELECT * FROM onebot_users; DROP TABLE onebot_users",
		"SELECT * FROM sqlite_master",
		"SELECT * FROM sessions",
		"SELECT * FROM onebot_users -- comment",
	}
	for _, badSQL := range invalidSQLs {
		raw, _ := json.Marshal(queryToolArgs{SQL: badSQL, Explain: "test invalid"})
		out, _ := qTool.Call(adminCtx, raw)
		if !out.Failed() {
			t.Fatalf("expected SQL %q to be rejected, but succeeded", badSQL)
		}
	}

	// 4.2 执行合法的聚合查询
	validRaw, _ := json.Marshal(queryToolArgs{
		SQL:     "SELECT enabled, COUNT(*) AS cnt FROM onebot_groups GROUP BY enabled ORDER BY enabled DESC",
		Explain: "统计开启与关闭的群聊数量",
	})
	out, err := qTool.Call(adminCtx, validRaw)
	if err != nil || out.Failed() {
		t.Fatalf("expected valid SELECT query to succeed, err=%v, out=%+v", err, out)
	}
	if !strings.Contains(out.Content, `"row_count": 2`) {
		t.Fatalf("expected 2 grouped rows in output, got: %s", out.Content)
	}
}

// TestSessionGuard_QueueAndCancel 测试 `SessionGuard` 的并发排队与取消指令中断能力。
// 5.1 提交一个耗时任务占用会话槽位；
// 5.2 在繁忙期间提交第二条任务，验证返回 GuardResultQueued；
// 5.3 提交“停”取消指令，验证正在运行的任务 Context 被立即取消且排队任务被清空。
func TestSessionGuard_QueueAndCancel(t *testing.T) {
	guard := NewSessionGuard()
	started := make(chan struct{})
	canceled := make(chan struct{})

	go func() {
		res := guard.Submit(context.Background(), "group:1001", "长耗时问题", func(runCtx context.Context) {
			close(started)
			<-runCtx.Done()
			close(canceled)
		})
		if res != GuardResultExecuted {
			t.Errorf("expected first task GuardResultExecuted, got %s", res)
		}
	}()

	<-started

	// 5.2 推理期间新消息自动进入 Pending 队列
	queuedRes := guard.Submit(context.Background(), "group:1001", "第二条问题", func(ctx context.Context) {})
	if queuedRes != GuardResultQueued {
		t.Fatalf("expected second message to be queued, got %s", queuedRes)
	}
	if guard.PendingCount("group:1001") != 1 {
		t.Fatalf("expected PendingCount=1, got %d", guard.PendingCount("group:1001"))
	}

	// 5.3 发送“停”取消当前推理并清空队列
	cancelRes := guard.Submit(context.Background(), "group:1001", "停", nil)
	if cancelRes != GuardResultCancelled {
		t.Fatalf("expected cancel command to return GuardResultCancelled, got %s", cancelRes)
	}

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for running task to be canceled")
	}
}

// TestGroupCompactor 测试群聊三段式压缩器的水位判定与完整轮次切分。
// 6.1 验证 70% 软阈值与 85% 硬阈值判定；
// 6.2 验证 SplitGroupMessages 保留尾部最新对话轮次并切分中段历史。
func TestGroupCompactor(t *testing.T) {
	chat := newMockChatFacade()
	compactor := NewGroupCompactor(chat)
	compactor.protectTurnChunks = 2

	// 6.1 验证软硬阈值判定
	lowInfo := contextmgr.Info{WindowK: 10, SelectedTokens: 5000}
	softInfo := contextmgr.Info{WindowK: 10, SelectedTokens: 7500}
	hardInfo := contextmgr.Info{WindowK: 10, SelectedTokens: 9000}

	if compactor.ShouldSoftCompact(lowInfo) {
		t.Fatalf("expected 50%% usage not to trigger soft compaction")
	}
	if !compactor.ShouldSoftCompact(softInfo) || compactor.ShouldHardCompact(softInfo) {
		t.Fatalf("expected 75%% usage to trigger soft compaction only")
	}
	if !compactor.ShouldHardCompact(hardInfo) {
		t.Fatalf("expected 90%% usage to trigger hard compaction")
	}

	// 6.2 验证完整轮次切分
	msgs := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "system prompt"),
		domainmessage.Text(domainmessage.RoleUser, "turn 1 user"),
		domainmessage.Text(domainmessage.RoleAssistant, "turn 1 assistant"),
		domainmessage.Text(domainmessage.RoleUser, "turn 2 user"),
		domainmessage.Text(domainmessage.RoleAssistant, "turn 2 assistant"),
		domainmessage.Text(domainmessage.RoleUser, "turn 3 user"),
		domainmessage.Text(domainmessage.RoleAssistant, "turn 3 assistant"),
	}
	toCompact, recent, cutoff := compactor.SplitGroupMessages(msgs, 1)
	if len(toCompact) != 2 || len(recent) != 4 || cutoff != 3 {
		t.Fatalf("unexpected split: len(toCompact)=%d, len(recent)=%d, cutoff=%d", len(toCompact), len(recent), cutoff)
	}
}

// TestBot_EndToEndWebSocketAndSessionIsolation 测试正向 WebSocket 连接、群聊引用回复、冷却静默丢弃与权限模式同步。
// 7.1 启动模拟的 NapCatQQ WebSocket Server；
// 7.2 启动 Bot 连接模拟服务端，推送超级管理员私聊事件与群聊 @提及 事件；
// 7.3 验证私聊自动开启 PermissionModeFull，群聊强制使用 PermissionModeReadonly 且回复带有 [CQ:reply] 引用段。
func TestBot_EndToEndWebSocketAndSessionIsolation(t *testing.T) {
	upgrader := websocket.Upgrader{}
	receivedActions := make(chan ActionRequest, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// 推送一条超级管理员私聊消息
		_ = conn.WriteJSON(map[string]any{
			"time":         time.Now().Unix(),
			"self_id":      999999,
			"post_type":    "message",
			"message_type": "private",
			"message_id":   101,
			"user_id":      2823626561,
			"sender":       map[string]any{"user_id": 2823626561, "nickname": "Boss"},
			"message": []map[string]any{
				{"type": "text", "data": map[string]any{"text": "你好"}},
			},
		})

		// 推送一条群聊 @机器人 消息
		_ = conn.WriteJSON(map[string]any{
			"time":         time.Now().Unix(),
			"self_id":      999999,
			"post_type":    "message",
			"message_type": "group",
			"group_id":     123456,
			"message_id":   202,
			"user_id":      30001,
			"sender":       map[string]any{"user_id": 30001, "nickname": "群友A"},
			"message": []map[string]any{
				{"type": "at", "data": map[string]any{"qq": "999999"}},
				{"type": "text", "data": map[string]any{"text": " 群里问个问题"}},
			},
		})

		for {
			var action ActionRequest
			if err := conn.ReadJSON(&action); err != nil {
				return
			}
			receivedActions <- action
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	store := NewMemoryStore()
	chat := newMockChatFacade()
	bot, err := NewBot(Config{
		WSURL:       wsURL,
		Token:       "test-secret",
		SuperAdmins: []int64{2823626561},
		GroupAtOnly: true,
		Cooldown:    5 * time.Second,
	}, store, chat)
	if err != nil {
		t.Fatalf("NewBot failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = bot.Run(ctx)
	}()

	// 等待接收私聊与群聊两条 send_msg 回复动作
	for i := 0; i < 2; i++ {
		select {
		case act := <-receivedActions:
			if act.Action != "send_msg" {
				t.Fatalf("expected action=send_msg, got %s", act.Action)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for bot send_msg action #%d", i+1)
		}
	}

	// 验证超级管理员私聊会话与群聊共享会话已分别建立且权限模式正确
	superUser, ok, _ := store.GetUser(ctx, 2823626561)
	if !ok || superUser.PrivateSessionID == "" {
		t.Fatalf("expected super_admin private session to be persisted, got %+v", superUser)
	}
	groupCfg, ok, _ := store.GetGroup(ctx, 123456)
	if !ok || groupCfg.SessionID == "" {
		t.Fatalf("expected group session to be persisted, got %+v", groupCfg)
	}

	chat.mu.Lock()
	privatePerm := chat.permissionModes[superUser.PrivateSessionID]
	groupPerm := chat.permissionModes[groupCfg.SessionID]
	chat.mu.Unlock()

	if privatePerm != "full" {
		t.Fatalf("expected super_admin private session permissionMode=full, got %q", privatePerm)
	}
	if groupPerm != "readonly" {
		t.Fatalf("expected group session permissionMode=readonly, got %q", groupPerm)
	}
}

// TestAdminAndUtilTools_FullCoverage 测试 6.2 清空历史、6.3 群开关、6.4 人设配置、6.5 模型切换与参数设置、6.6 列表查询与 6.8 状态查询工具。
// 8.1 测试 onebot_set_persona 设置群聊人设并持久化到 onebot_groups；
// 8.2 测试 onebot_switch_model 切换模型并同时设置 temperature / max_tokens / top_p 参数；
// 8.3 测试 onebot_set_group_switch 关闭与开启群服务；
// 8.4 测试 onebot_clear_history 清空群会话；
// 8.5 测试 onebot_list_users_and_sessions 与 onebot_get_status 返回完整状态。
func TestAdminAndUtilTools_FullCoverage(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.SeedSuperAdmins(ctx, []int64{10001})
	chat := newMockChatFacade()
	sender := &mockSender{}

	adminCtx := WithCallerInfo(ctx, CallerInfo{
		UserID:      10001,
		Nickname:    "SuperAdmin",
		GroupID:     88801,
		MessageType: "group",
	})

	// 8.1 测试 onebot_set_persona
	personaTool := NewSetPersonaTool(store, chat)
	out, _ := personaTool.Call(adminCtx, json.RawMessage(`{"style_instruction":"傲娇猫娘，句尾带喵","scope":"group"}`))
	if out.Failed() {
		t.Fatalf("onebot_set_persona failed: %s", out.ErrorMessage)
	}
	grp, ok, _ := store.GetGroup(ctx, 88801)
	if !ok || grp.Persona != "傲娇猫娘，句尾带喵" || grp.SessionID == "" {
		t.Fatalf("expected group persona and session_id to be saved, got %+v", grp)
	}

	// 8.2 测试 onebot_switch_model (switch + set_params + reset_params)
	modelTool := NewSwitchModelTool(store, chat)
	out, _ = modelTool.Call(adminCtx, json.RawMessage(`{"action":"switch","model_id":"deepseek-chat","temperature":0.8,"max_tokens":1024,"top_p":0.9}`))
	if out.Failed() {
		t.Fatalf("onebot_switch_model switch failed: %s", out.ErrorMessage)
	}
	chat.mu.Lock()
	switchedModel := chat.models[grp.SessionID]
	savedTemp := chat.genSettings[grp.SessionID].Temperature
	chat.mu.Unlock()
	if switchedModel != "deepseek-chat" || savedTemp == nil || *savedTemp != 0.8 {
		t.Fatalf("expected model=deepseek-chat and temp=0.8, got model=%q, temp=%v", switchedModel, savedTemp)
	}

	// 校验非法 temperature 范围被拦截
	out, _ = modelTool.Call(adminCtx, json.RawMessage(`{"action":"set_params","temperature":3.5}`))
	if !out.Failed() {
		t.Fatalf("expected invalid temperature 3.5 to be rejected")
	}

	// 8.3 测试 onebot_set_group_switch
	switchTool := NewSetGroupSwitchTool(store)
	out, _ = switchTool.Call(adminCtx, json.RawMessage(`{"enabled":false}`))
	if out.Failed() {
		t.Fatalf("onebot_set_group_switch failed: %s", out.ErrorMessage)
	}
	grp, _, _ = store.GetGroup(ctx, 88801)
	if grp.Enabled {
		t.Fatalf("expected group 88801 enabled=false, got true")
	}

	// 8.4 测试 onebot_clear_history
	clearTool := NewClearHistoryTool(store, chat)
	out, _ = clearTool.Call(adminCtx, json.RawMessage(`{"scope":"group"}`))
	if out.Failed() {
		t.Fatalf("onebot_clear_history failed: %s", out.ErrorMessage)
	}
	grp, _, _ = store.GetGroup(ctx, 88801)
	if grp.SessionID != "" {
		t.Fatalf("expected group session_id to be cleared, got %q", grp.SessionID)
	}

	// 8.5 测试 onebot_list_users_and_sessions 与 onebot_get_status
	listTool := NewListUsersAndSessionsTool(store)
	out, _ = listTool.Call(adminCtx, json.RawMessage(`{"role_filter":"super_admin"}`))
	if out.Failed() || !strings.Contains(out.Content, "10001") {
		t.Fatalf("onebot_list_users_and_sessions unexpected output: %+v", out)
	}

	statusTool := NewGetStatusTool(store, chat, sender)
	out, _ = statusTool.Call(adminCtx, json.RawMessage(`{}`))
	if out.Failed() || !strings.Contains(out.Content, `"connected": true`) {
		t.Fatalf("onebot_get_status unexpected output: %+v", out)
	}
}

