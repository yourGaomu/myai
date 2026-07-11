package service

import (
	"context"
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
	if len(records) > 0 {
		return MessageListItems(records), nil
	}
	return MessageListItems(s.memoryMessages(sessionID)), nil
}

func (s MessageQueryService) HistoryMeta(ctx context.Context, sessionID string) (sessionresult.MessageHistoryMeta, error) {
	if s.Store == nil {
		return MessageHistoryMetaResultFromRecord(MessageHistoryMetaFromRecords(sessionID, s.memoryMessages(sessionID))), nil
	}
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return sessionresult.MessageHistoryMeta{}, err
	}
	record, err := s.Store.GetMessageHistoryMeta(ctx, sessionID)
	if err != nil {
		return sessionresult.MessageHistoryMeta{}, err
	}
	return MessageHistoryMetaResultFromRecord(record), nil
}

func (s MessageQueryService) ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]sessionresult.MessageListItem, bool, error) {
	if s.Store == nil {
		records, fullSyncRequired, err := MessagesAfterID(s.memoryMessages(sessionID), afterMessageID, limit)
		if err != nil {
			return nil, false, err
		}
		return MessageListItems(records), fullSyncRequired, nil
	}
	if _, err := s.Store.GetSession(ctx, sessionID); err != nil {
		return nil, false, err
	}
	records, fullSyncRequired, err := s.Store.ListMessagesAfter(ctx, sessionID, afterMessageID, limit)
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
	meta := repository.MessageHistoryMeta{
		SessionID:      sessionID,
		MessageCount:   int64(len(records)),
		HistoryVersion: int64(len(records)),
	}
	if len(records) > 0 {
		last := records[len(records)-1]
		meta.LastMessageID = last.ID
		meta.LastMessageCreatedAt = &last.CreatedAt
	}
	return meta
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
