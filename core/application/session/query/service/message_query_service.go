package service

import (
	"context"
	"sort"
	"strings"

	queryapi "myai/core/application/session/query/api"
	queryport "myai/core/application/session/query/port"
	sessionresult "myai/core/application/session/result"
	repository "myai/core/port/repository"
)

type MessageQueryService struct {
	// 查询优先返回内存消息以包含尚未异步落库的最新内容；历史分页再使用持久层。
	Store         queryport.MessageQueryStore
	Memory        queryport.MemorySessionSource
	MemoryRecords queryport.MemoryMessageRecordMapper
}

var _ queryapi.MessageQueryService = MessageQueryService{}

func (s MessageQueryService) ListMessages(ctx context.Context, sessionID string) ([]sessionresult.MessageListItem, error) {
	if s.Store == nil {
		return MessageListItems(s.memoryMessages(sessionID)), nil
	}
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return nil, err
	}

	records, err := s.Store.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return MessageListItems(mergeMessageRecords(records, s.memoryMessages(sessionID))), nil
}

func (s MessageQueryService) HistoryMeta(ctx context.Context, sessionID string) (sessionresult.MessageHistoryMeta, error) {
	if s.Store == nil {
		return MessageHistoryMetaResultFromRecord(MessageHistoryMetaFromRecords(sessionID, s.memoryMessages(sessionID))), nil
	}
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return sessionresult.MessageHistoryMeta{}, err
	}
	memory := s.memoryMessages(sessionID)
	if len(memory) == 0 {
		meta, err := s.Store.GetMessageHistoryMeta(ctx, sessionID)
		if err != nil {
			return sessionresult.MessageHistoryMeta{}, err
		}
		return MessageHistoryMetaResultFromRecord(meta), nil
	}
	records, err := s.Store.ListMessages(ctx, sessionID)
	if err != nil {
		return sessionresult.MessageHistoryMeta{}, err
	}
	return MessageHistoryMetaResultFromRecord(MessageHistoryMetaFromRecords(sessionID, mergeMessageRecords(records, memory))), nil
}

func (s MessageQueryService) ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]sessionresult.MessageListItem, bool, error) {
	if s.Store == nil {
		records, fullSyncRequired, err := MessagesAfterID(visibleMessageRecords(s.memoryMessages(sessionID)), afterMessageID, limit)
		if err != nil {
			return nil, false, err
		}
		return MessageListItems(records), fullSyncRequired, nil
	}
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return nil, false, err
	}
	memory := s.memoryMessages(sessionID)
	if len(memory) == 0 {
		records, fullSyncRequired, err := s.Store.ListMessagesAfter(ctx, sessionID, afterMessageID, limit)
		if err != nil {
			return nil, false, err
		}
		return MessageListItems(visibleMessageRecords(records)), fullSyncRequired, nil
	}
	records, err := s.Store.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	records, fullSyncRequired, err := MessagesAfterID(mergeMessageRecords(records, memory), afterMessageID, limit)
	return MessageListItems(records), fullSyncRequired, err
}

func (s MessageQueryService) memoryMessages(sessionID string) []repository.MessageRecord {
	if s.Memory == nil {
		return nil
	}
	current, err := s.Memory.GetSession(sessionID)
	if err != nil || s.MemoryRecords == nil {
		return nil
	}
	return s.MemoryRecords.MemoryMessages(current)
}

func MessageHistoryMetaFromRecords(sessionID string, records []repository.MessageRecord) repository.MessageHistoryMeta {
	records = visibleMessageRecords(records)
	meta := repository.MessageHistoryMeta{
		SessionID:      sessionID,
		MessageCount:   int64(len(records)),
		HistoryVersion: historyVersion(records),
	}
	if len(records) > 0 {
		last := records[len(records)-1]
		meta.LastMessageID = last.ID
		meta.LastMessageCreatedAt = &last.CreatedAt
	}
	return meta
}

// historyVersion returns a monotonic durable version when records carry the
// mapper-generated sequence. Legacy records without a sequence retain the
// count fallback for compatibility.
func historyVersion(records []repository.MessageRecord) int64 {
	version := int64(len(records))
	for _, record := range records {
		if record.Sequence > version {
			version = record.Sequence
		}
	}
	return version
}

func visibleMessageRecords(records []repository.MessageRecord) []repository.MessageRecord {
	visible := make([]repository.MessageRecord, 0, len(records))
	for _, record := range records {
		if record.SyntheticReason != "" {
			continue
		}
		visible = append(visible, record)
	}
	return visible
}

// mergeMessageRecords combines the durable transcript with the live in-memory
// session. The in-memory session may contain messages that have not reached the
// asynchronous persistence queue yet. Durable IDs are preferred when a record
// is already persisted; memory-only records are merged and then ordered by the
// durable cursor when available. This keeps list, metadata, and delta reads on
// the same snapshot.
func mergeMessageRecords(stored []repository.MessageRecord, memory []repository.MessageRecord) []repository.MessageRecord {
	stored = visibleMessageRecords(stored)
	memory = visibleMessageRecords(memory)
	if len(memory) == 0 {
		return stored
	}
	if len(stored) == 0 {
		return memory
	}

	merged := append([]repository.MessageRecord(nil), stored...)
	matched := make([]bool, len(stored))
	for _, candidate := range memory {
		matchedIndex := -1
		for index, existing := range stored {
			if matched[index] {
				continue
			}
			if candidate.ID != "" && existing.ID == candidate.ID {
				matchedIndex = index
				break
			}
			if messageRecordEquivalent(existing, candidate) {
				matchedIndex = index
				break
			}
		}
		if matchedIndex >= 0 {
			matched[matchedIndex] = true
			continue
		}
		merged = append(merged, candidate)
	}
	sort.SliceStable(merged, func(left, right int) bool {
		return messageRecordLess(merged[left], merged[right])
	})
	return merged
}

func messageRecordLess(left repository.MessageRecord, right repository.MessageRecord) bool {
	if left.Sequence != 0 && right.Sequence != 0 && left.Sequence != right.Sequence {
		return left.Sequence < right.Sequence
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		if !left.CreatedAt.IsZero() && !right.CreatedAt.IsZero() {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		// A legacy record with no ordering metadata stays in its original
		// stable position relative to a record whose metadata is incomplete.
		return false
	}
	if left.Sequence == 0 || right.Sequence == 0 {
		return false
	}
	return left.ID < right.ID
}

func messageRecordEquivalent(left repository.MessageRecord, right repository.MessageRecord) bool {
	return (left.SessionID == right.SessionID || left.SessionID == "" || right.SessionID == "") &&
		left.Role == right.Role &&
		left.Content == right.Content &&
		left.Reasoning == right.Reasoning &&
		left.ToolCallID == right.ToolCallID &&
		left.ToolName == right.ToolName &&
		left.ToolArguments == right.ToolArguments &&
		left.ToolError == right.ToolError &&
		left.ToolStatus == right.ToolStatus &&
		left.ToolErrorCode == right.ToolErrorCode &&
		left.SyntheticReason == right.SyntheticReason
}

func MessagesAfterID(records []repository.MessageRecord, afterMessageID string, limit int) ([]repository.MessageRecord, bool, error) {
	if limit <= 0 || limit > 300 {
		limit = 100
	}
	afterMessageID = strings.TrimSpace(afterMessageID)
	if afterMessageID == "" {
		if len(records) <= limit {
			return records, false, nil
		}
		return records[:limit], false, nil
	}

	start := -1
	for index, record := range records {
		if record.ID == afterMessageID {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return nil, true, nil
	}
	if start >= len(records) {
		return nil, false, nil
	}
	end := start + limit
	if end > len(records) {
		end = len(records)
	}
	return records[start:end], false, nil
}
