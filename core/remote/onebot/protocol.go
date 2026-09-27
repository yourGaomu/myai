package onebot

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Sender 对应 OneBot v11 事件中的 sender 字段，包含发送者的昵称、群名片与群内角色。
// 1.1 UserID：发送者 QQ 号；
// 1.2 Nickname：QQ 昵称；
// 1.3 Card：群名片（仅群聊存在，若非空优先作为显示名称）；
// 1.4 Role：QQ 群内置角色（owner / admin / member，注意区别于机器人自身的 RBAC Role）。
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
	Role     string `json:"role"`
}

// DisplayName 返回发送者的最佳显示名称。
// 1.1 优先使用群名片 Card；
// 1.2 若群名片为空则回退至 QQ 昵称 Nickname；
// 1.3 若均为空则回退至 "QQ_<UserID>"。
func (s Sender) DisplayName(fallbackUserID int64) string {
	if card := strings.TrimSpace(s.Card); card != "" {
		return card
	}
	if nick := strings.TrimSpace(s.Nickname); nick != "" {
		return nick
	}
	uid := s.UserID
	if uid == 0 {
		uid = fallbackUserID
	}
	return fmt.Sprintf("QQ_%d", uid)
}

// Segment 对应 OneBot v11 数组消息段格式中的单个消息段（如 text、at、reply、image）。
// 2.1 Type：消息段类型（"text" | "at" | "reply" | "image" | "face" 等）；
// 2.2 Data：消息段载荷键值映射（例如 text 段包含 "text"，at 段包含 "qq"，reply 段包含 "id"）。
type Segment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// TextSegment 构造一个纯文本消息段。
// 2.1 设置 Type = "text"，Data["text"] = text。
func TextSegment(text string) Segment {
	return Segment{
		Type: "text",
		Data: map[string]any{"text": text},
	}
}

// ReplySegment 构造一个引用回复指定 message_id 的消息段。
// 2.1 设置 Type = "reply"，Data["id"] = strconv.FormatInt(int64(messageID), 10)。
func ReplySegment(messageID int32) Segment {
	return Segment{
		Type: "reply",
		Data: map[string]any{"id": strconv.FormatInt(int64(messageID), 10)},
	}
}

// AtSegment 构造一个 @指定 QQ 号的消息段。
// 2.1 设置 Type = "at"，Data["qq"] = strconv.FormatInt(userID, 10)。
func AtSegment(userID int64) Segment {
	return Segment{
		Type: "at",
		Data: map[string]any{"qq": strconv.FormatInt(userID, 10)},
	}
}

// Event 对应 NapCatQQ 通过正向 WebSocket 推送的 OneBot v11 顶层数据包。
// 3.1 PostType：事件大类（"message" | "meta_event" | "notice" | "request"）；
// 3.2 MetaEventType：元事件子类（如 "heartbeat" | "lifecycle"）；
// 3.3 MessageType：消息类型（"private" 私聊 | "group" 群聊）；
// 3.4 SelfID：机器人自身的 QQ 号；
// 3.5 UserID / GroupID / MessageID：发送者 QQ 号、群号与消息 ID；
// 3.6 Message：原始 JSON 消息内容（支持数组格式 []Segment，同时兼容字符串格式回退）；
// 3.7 Status / RetCode / Echo：当数据包为 API 调用响应（如 send_msg 回包）时的状态字段。
type Event struct {
	Time          int64           `json:"time"`
	SelfID        int64           `json:"self_id"`
	PostType      string          `json:"post_type"`
	MetaEventType string          `json:"meta_event_type"`
	MessageType   string          `json:"message_type"`
	SubType       string          `json:"sub_type"`
	MessageID     int32           `json:"message_id"`
	UserID        int64           `json:"user_id"`
	GroupID       int64           `json:"group_id"`
	RawMessage    string          `json:"raw_message"`
	Sender        Sender          `json:"sender"`
	Message       json.RawMessage `json:"message"`

	// API 动作响应字段（当 PostType 为空且 Echo 非空时表示是动作回包）
	Status  string `json:"status,omitempty"`
	RetCode int    `json:"retcode,omitempty"`
	Echo    string `json:"echo,omitempty"`
}

// ParseSegments 将 Event.Message 解析为标准的消息段切片 []Segment。
// 4.1 若 Message 为空，但 RawMessage 非空，则包装为单条 text 消息段；
// 4.2 优先按 OneBot v11 数组格式 `[]Segment` 反序列化；
// 4.3 若反序列化数组失败（例如误配为 string 格式），则尝试按字符串反序列化并包装为 text 消息段。
func (e Event) ParseSegments() ([]Segment, error) {
	raw := strings.TrimSpace(string(e.Message))
	if raw == "" || raw == "null" {
		if strings.TrimSpace(e.RawMessage) != "" {
			return []Segment{TextSegment(e.RawMessage)}, nil
		}
		return nil, nil
	}

	// 4.2 优先解析为数组格式 []Segment
	if strings.HasPrefix(raw, "[") {
		var segments []Segment
		if err := json.Unmarshal(e.Message, &segments); err != nil {
			return nil, fmt.Errorf("unmarshal message segments failed: %w", err)
		}
		return segments, nil
	}

	// 4.3 兼容字符串格式回退
	var text string
	if err := json.Unmarshal(e.Message, &text); err == nil {
		return []Segment{TextSegment(text)}, nil
	}
	return []Segment{TextSegment(raw)}, nil
}

// ParsedMessage 保存对一条 OneBot 消息段数组进行语义解析后的结果。
// 5.1 CleanText：清洗并转换 @提及 后的纯文本内容（可直接拼入大模型 Prompt）；
// 5.2 MentionedBot：该消息是否显式 @ 了机器人自身（用于群聊触发判定）；
// 5.3 MentionedUsers：消息中 @ 提及的其他用户 QQ 号列表；
// 5.4 ReplyToMessageID：若消息包含引用回复段，则记录被引用的消息 ID。
type ParsedMessage struct {
	CleanText        string
	MentionedBot     bool
	MentionedUsers   []int64
	ReplyToMessageID string
}

// ExtractMessageContent 解析消息段数组，识别 @机器人 并将其余 @提及 转换为大模型可读文本。
// 5.1 遍历所有 Segment，根据 Type 分支处理：
//   - "text"：直接追加文本内容；
//   - "at"：提取 data["qq"]，若等于 selfID 则标记 MentionedBot = true（不拼入正文以免干扰模型）；
//     若为其他 QQ 号，则转换为 ` [提及用户 QQ: <qq>] ` 拼入正文，让大模型直接获知被提及人 QQ 号；
//   - "image"：转换为 `[图片]` 占位标记（如有 url 则附带简短说明），过滤无意义表情刷屏；
//   - "reply"：记录被引用回复的消息 ID；
// 5.2 对最终拼接的字符串进行首尾空白清理并返回。
func ExtractMessageContent(segments []Segment, selfID int64) ParsedMessage {
	var builder strings.Builder
	var parsed ParsedMessage
	selfStr := strconv.FormatInt(selfID, 10)

	for _, seg := range segments {
		switch strings.ToLower(strings.TrimSpace(seg.Type)) {
		case "text":
			text := segmentDataString(seg.Data, "text")
			builder.WriteString(text)

		case "at":
			qqStr := strings.TrimSpace(segmentDataString(seg.Data, "qq"))
			if qqStr == "" {
				continue
			}
			if selfID > 0 && qqStr == selfStr {
				parsed.MentionedBot = true
				continue
			}
			if strings.EqualFold(qqStr, "all") {
				builder.WriteString(" [提及全体成员] ")
				continue
			}
			if qqNum, err := strconv.ParseInt(qqStr, 10, 64); err == nil && qqNum > 0 {
				parsed.MentionedUsers = append(parsed.MentionedUsers, qqNum)
			}
			nameHint := strings.TrimSpace(segmentDataString(seg.Data, "name"))
			if nameHint != "" {
				builder.WriteString(fmt.Sprintf(" [提及用户 QQ: %s (昵称: %s)] ", qqStr, nameHint))
			} else {
				builder.WriteString(fmt.Sprintf(" [提及用户 QQ: %s] ", qqStr))
			}

		case "reply":
			parsed.ReplyToMessageID = strings.TrimSpace(segmentDataString(seg.Data, "id"))

		case "image":
			builder.WriteString(" [图片] ")
		}
	}

	parsed.CleanText = strings.TrimSpace(builder.String())
	return parsed
}

// segmentDataString 安全地从 Segment.Data 映射中提取字符串值（兼容 string、float64、json.Number、int64）。
// 6.1 检查 data map 是否为空及键是否存在；
// 6.2 根据动态类型转换为对应的字符串表示。
func segmentDataString(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	raw, ok := data[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

// ActionRequest 对应向 NapCatQQ 正向 WebSocket 发送的 API 调用数据包（如 send_msg）。
// 7.1 Action：API 动作名称（例如 "send_msg"）；
// 7.2 Params：动作参数结构体；
// 7.3 Echo：请求追踪标识，用于区分异步回包。
type ActionRequest struct {
	Action string `json:"action"`
	Params any    `json:"params"`
	Echo   string `json:"echo,omitempty"`
}

// SendMsgParams 对应 OneBot v11 `send_msg` 动作的参数结构。
// 7.1 MessageType："private" 或 "group"；
// 7.2 UserID：私聊目标 QQ 号（私聊必填）；
// 7.3 GroupID：群聊目标群号（群聊必填）；
// 7.4 Message：要发送的消息段数组 `[]Segment`。
type SendMsgParams struct {
	MessageType string    `json:"message_type"`
	UserID      int64     `json:"user_id,omitempty"`
	GroupID     int64     `json:"group_id,omitempty"`
	Message     []Segment `json:"message"`
}

// BuildPrivateMsgAction 构造向指定 QQ 用户发送私聊文本的 send_msg 动作包。
// 8.1 组装包含单条 TextSegment 的 SendMsgParams。
func BuildPrivateMsgAction(userID int64, text string, echo string) ActionRequest {
	return ActionRequest{
		Action: "send_msg",
		Echo:   echo,
		Params: SendMsgParams{
			MessageType: "private",
			UserID:      userID,
			Message:     []Segment{TextSegment(text)},
		},
	}
}

// BuildGroupMsgAction 构造向指定 QQ 群发送消息（可选带 reply 引用原消息）的 send_msg 动作包。
// 8.1 若 replyMessageID > 0，则在消息段数组头部插入 ReplySegment；
// 8.2 追加正文 TextSegment 并返回 ActionRequest。
func BuildGroupMsgAction(groupID int64, replyMessageID int32, text string, echo string) ActionRequest {
	segments := make([]Segment, 0, 2)
	if replyMessageID != 0 {
		segments = append(segments, ReplySegment(replyMessageID))
	}
	segments = append(segments, TextSegment(text))
	return ActionRequest{
		Action: "send_msg",
		Echo:   echo,
		Params: SendMsgParams{
			MessageType: "group",
			GroupID:     groupID,
			Message:     segments,
		},
	}
}
