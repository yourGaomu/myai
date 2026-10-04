package pendinginput

import (
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	generationport "myai/core/application/chat/generation/port"
	domaingeneration "myai/core/domain/generation"
	domainsubagent "myai/core/domain/subagent"
)

const (
	MaxPendingMessages = 8
	MaxPendingRunes    = 8000
)

type Queue struct {
	mu       sync.Mutex
	messages map[string][]domaingeneration.PendingTurnInputItem
	// inflight is the process-local claim set. Durable delivery state lives in
	// AgentMessageRepository; this set prevents a retry from being enqueued
	// again while the current turn is still consuming the message.
	inflight     map[string]map[string]domaingeneration.PendingTurnInputItem
	acknowledger generationport.PendingInputAcknowledger
}

func NewQueue() *Queue {
	return &Queue{
		messages: make(map[string][]domaingeneration.PendingTurnInputItem),
		inflight: make(map[string]map[string]domaingeneration.PendingTurnInputItem),
	}
}

var _ generationport.PendingTurnInput = (*Queue)(nil)
var _ generationport.IdentifiedPendingTurnInput = (*Queue)(nil)
var _ generationport.PendingInputConfigurer = (*Queue)(nil)
var _ generationport.PendingInputReleaser = (*Queue)(nil)

func (q *Queue) Enqueue(sessionID, content string) error {
	sessionID = strings.TrimSpace(sessionID)
	content = strings.TrimSpace(content)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if content == "" {
		return errors.New("pending turn input is empty")
	}
	if utf8.RuneCountInString(content) > MaxPendingRunes {
		return errors.New("pending turn input is too long")
	}
	if q == nil {
		return errors.New("pending turn input queue is nil")
	}
	return q.enqueue(sessionID, "", content)
}

func (q *Queue) EnqueueIdentified(sessionID, messageID, content string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("pending message id is empty")
	}
	return q.enqueue(sessionID, messageID, content)
}

func (q *Queue) EnqueueAgentMessage(sessionID string, message domainsubagent.AgentMessage) error {
	// 1. 先校验完整消息信封，确保接收方、消息类型和投递模式都有效。
	if err := message.Validate(); err != nil {
		return err
	}
	// 2. 再把结构化元数据复制到短暂队列，等待 AgentLoop 领取。
	return q.enqueueItem(sessionID, message.ID, message.Content, domaingeneration.PendingTurnInputItem{
		ID: message.ID, Content: message.Content, SourceKind: string(message.Kind), SourceTaskID: message.SourceTaskID,
		AuthorAgentID: message.AuthorAgentID, RecipientAgentID: message.RecipientAgentID,
		ParentTurnID: message.ParentTurnID, RootAgentID: message.RootAgentID, Trigger: string(message.Trigger),
		Sequence: message.Sequence, CreatedAt: message.CreatedAt,
	})
}

func (q *Queue) enqueue(sessionID, messageID, content string) error {
	return q.enqueueItem(sessionID, messageID, content, domaingeneration.PendingTurnInputItem{ID: messageID, Content: content})
}

func (q *Queue) enqueueItem(sessionID, messageID, content string, item domaingeneration.PendingTurnInputItem) error {
	// 1. 入队前统一清理并限制消息大小，避免无效数据进入 mailbox。
	sessionID = strings.TrimSpace(sessionID)
	content = strings.TrimSpace(content)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if content == "" {
		return errors.New("pending turn input is empty")
	}
	if utf8.RuneCountInString(content) > MaxPendingRunes {
		return errors.New("pending turn input is too long")
	}
	if q == nil {
		return errors.New("pending turn input queue is nil")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	// 2. 同一个事件 ID 只能对应同一个请求；内容变化必须显式报冲突。
	for _, existing := range q.messages[sessionID] {
		if messageID == "" || existing.ID != messageID {
			continue
		}
		if !samePendingItem(existing, item) {
			return errors.New("pending message id was already used with different request")
		}
		return nil
	}
	if messageID != "" {
		// 3. 已经被当前 turn 领取的消息也要参与幂等判断，防止重试再次排队。
		if existing, ok := q.inflight[sessionID][messageID]; ok {
			if !samePendingItem(existing, item) {
				return errors.New("pending message id is currently in flight with different request")
			}
			return nil
		}
	}
	if len(q.messages[sessionID]) >= MaxPendingMessages {
		return errors.New("pending turn input queue is full")
	}
	// 4. 只有通过前面的身份和容量检查后，消息才进入待领取队列。
	item.ID = messageID
	item.Content = content
	q.messages[sessionID] = append(q.messages[sessionID], item)
	return nil
}

func (q *Queue) Drain(sessionID string) []string {
	if q == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.messages[sessionID]
	delete(q.messages, sessionID)
	if len(items) == 0 {
		return nil
	}
	contents := make([]string, 0, len(items))
	for _, item := range items {
		contents = append(contents, item.Content)
	}
	return contents
}

func (q *Queue) DrainIdentified(sessionID string) []domaingeneration.PendingTurnInputItem {
	if q == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.messages[sessionID]
	delete(q.messages, sessionID)
	if len(items) == 0 {
		return nil
	}
	// 1. 从待处理队列移出并建立 in-flight 记录，表示当前 turn 已经领取消息。
	if q.inflight[sessionID] == nil {
		q.inflight[sessionID] = make(map[string]domaingeneration.PendingTurnInputItem)
	}
	for _, item := range items {
		if item.ID != "" {
			q.inflight[sessionID][item.ID] = item
		}
	}
	// 2. 返回快照，后续成功时确认，失败时按原顺序回滚。
	return append([]domaingeneration.PendingTurnInputItem(nil), items...)
}

// RequeueIdentified restores a claimed batch in its original order. Existing
// IDs are ignored so a retry cannot duplicate a durable agent message.
func (q *Queue) RequeueIdentified(sessionID string, items []domaingeneration.PendingTurnInputItem) error {
	if q == nil {
		return errors.New("pending turn input queue is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is empty")
	}
	if len(items) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	// 1. 先检查当前队列和 in-flight 中是否存在同 ID 的内容冲突。
	current := q.messages[sessionID]
	seen := make(map[string]struct{}, len(current)+len(items))
	for _, item := range current {
		if item.ID != "" {
			seen[item.ID] = struct{}{}
		}
	}
	restored := make([]domaingeneration.PendingTurnInputItem, 0, len(items)+len(current))
	for _, item := range items {
		item.Content = strings.TrimSpace(item.Content)
		if item.Content == "" {
			return errors.New("pending turn input is empty")
		}
		if utf8.RuneCountInString(item.Content) > MaxPendingRunes {
			return errors.New("pending turn input is too long")
		}
		if item.ID != "" {
			if existing, exists := q.inflight[sessionID][item.ID]; exists && !samePendingItem(existing, item) {
				return errors.New("cannot requeue pending message with conflicting in-flight request")
			}
			if existing := findPendingItem(current, item.ID); existing != nil {
				if !samePendingItem(*existing, item) {
					return errors.New("cannot requeue pending message with conflicting queued request")
				}
				delete(q.inflight[sessionID], item.ID)
				continue
			}
			if _, exists := seen[item.ID]; exists {
				delete(q.inflight[sessionID], item.ID)
				continue
			}
			seen[item.ID] = struct{}{}
		}
		if item.ID != "" {
			delete(q.inflight[sessionID], item.ID)
		}
		restored = append(restored, item)
	}
	// 2. 通过检查的消息重新放回队首，下一次 turn 可以继续尝试投递。
	if len(restored)+len(current) > MaxPendingMessages {
		return errors.New("pending turn input queue is full")
	}
	restored = append(restored, current...)
	q.messages[sessionID] = restored
	return nil
}

func (q *Queue) SetAcknowledger(acknowledger generationport.PendingInputAcknowledger) {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.acknowledger = acknowledger
	q.mu.Unlock()
}

func (q *Queue) Acknowledge(messageID string) error {
	if q == nil || strings.TrimSpace(messageID) == "" {
		return nil
	}
	q.mu.Lock()
	acknowledger := q.acknowledger
	q.mu.Unlock()
	if acknowledger == nil {
		return nil
	}
	// 1. 先让持久化适配器完成 claim/complete，只有成功后才能清理本地 in-flight。
	if err := acknowledger.Acknowledge(messageID); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	// 2. 本地确认只删除对应消息，不影响同一会话中的其他待处理事件。
	for sessionID, items := range q.inflight {
		delete(items, messageID)
		if len(items) == 0 {
			delete(q.inflight, sessionID)
		}
	}
	return nil
}

func (q *Queue) HasPending(sessionID string) bool {
	if q == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages[sessionID]) > 0
}

func (q *Queue) HasImmediatePending(sessionID string) bool {
	if q == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range q.messages[sessionID] {
		if item.Trigger == string(domainsubagent.AgentMessageTriggerSteer) {
			return true
		}
	}
	return false
}

func samePendingItem(left, right domaingeneration.PendingTurnInputItem) bool {
	return left.ID == right.ID && left.Content == right.Content &&
		left.SourceKind == right.SourceKind && left.SourceTaskID == right.SourceTaskID &&
		left.AuthorAgentID == right.AuthorAgentID && left.RecipientAgentID == right.RecipientAgentID &&
		left.ParentTurnID == right.ParentTurnID && left.RootAgentID == right.RootAgentID && left.Trigger == right.Trigger &&
		left.Sequence == right.Sequence
}

func findPendingItem(items []domaingeneration.PendingTurnInputItem, id string) *domaingeneration.PendingTurnInputItem {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}
