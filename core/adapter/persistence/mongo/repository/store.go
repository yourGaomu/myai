package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongomapper "myai/core/adapter/persistence/mongo/mapper"
	"myai/core/adapter/persistence/mongo/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainmodel "myai/core/domain/model"
	repository "myai/core/port/repository"
)

const (
	sessionsCollection     = "sessions"
	messagesCollection     = "messages"
	modelConfigsCollection = "model_configs"
	assetsCollection       = "assets"
)

type Store struct {
	// Store 实现应用层仓库接口；所有 Mongo 通用 CRUD 通过 template 统一封装。
	template mongotemplate.Operations
}

func New(client *gomongo.Client, database string) *Store {
	if client == nil || database == "" {
		return NewWithTemplate(mongotemplate.New(nil))
	}
	return NewWithTemplate(mongotemplate.New(client.Database(database)))
}

func NewWithTemplate(template mongotemplate.Operations) *Store {
	if template == nil {
		template = mongotemplate.New(nil)
	}
	return &Store{template: template}
}

func (m *Store) GetSession(ctx context.Context, sessionID string) (repository.SessionRecord, error) {
	var session po.SessionDocument
	err := m.template.FindOne(ctx, sessionsCollection, bson.M{
		"_id": sessionID,
		"$or": bson.A{
			bson.M{"deleted": bson.M{"$exists": false}},
			bson.M{"deleted": false},
		},
	}, &session)
	if err != nil {
		return repository.SessionRecord{}, repositoryError(err)
	}
	return mongomapper.SessionRecordFromDocument(session), nil
}

func (m *Store) SaveSession(ctx context.Context, session repository.SessionRecord) error {
	document := mongomapper.SessionDocumentFromRecord(session)
	// 使用 upsert 保证新会话和已有会话走同一写入路径；created_at 只在首次插入时设置。
	_, err := m.template.UpdateOne(
		ctx,
		sessionsCollection,
		bson.M{"_id": document.ID},
		bson.M{
			"$set": bson.M{
				"model":              document.Model,
				"agent_mode":         document.AgentMode,
				"permission_mode":    document.PermissionMode,
				"context_window_k":   document.ContextWindowK,
				"summary":            document.Summary,
				"compacted_messages": document.CompactedMessages,
				"compacted_at":       document.CompactedAt,
				"title":              document.Title,
				"usage":              document.Usage,
				"last_usage":         document.LastUsage,
				"current_plan":       document.CurrentPlan,
				"updated_at":         document.UpdatedAt,
			},
			"$setOnInsert": bson.M{
				"_id":        document.ID,
				"deleted":    false,
				"created_at": document.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (m *Store) MarkSessionDeleted(ctx context.Context, sessionID string, deletedAt time.Time) error {
	_, err := m.template.UpdateOne(
		ctx,
		sessionsCollection,
		bson.M{"_id": sessionID},
		bson.M{"$set": bson.M{
			"deleted":    true,
			"deleted_at": deletedAt,
			"updated_at": deletedAt,
		}},
	)
	return err
}

func (m *Store) MarkSessionRestored(ctx context.Context, sessionID string, restoredAt time.Time) error {
	_, err := m.template.UpdateOne(
		ctx,
		sessionsCollection,
		bson.M{"_id": sessionID, "deleted": true},
		bson.M{
			"$set": bson.M{
				"deleted":    false,
				"updated_at": restoredAt,
			},
			"$unset": bson.M{
				"deleted_at": "",
			},
		},
	)
	return err
}

func (m *Store) SaveConfig(ctx context.Context, model domainmodel.Config) error {
	document := mongomapper.ModelConfigDocumentFromDomain(model)
	_, err := m.template.UpdateOne(
		ctx,
		modelConfigsCollection,
		bson.M{"_id": document.ID},
		bson.M{
			"$set": bson.M{
				"name":       document.Name,
				"provider":   document.Provider,
				"base_url":   document.BaseURL,
				"api_key":    document.APIKey,
				"model_name": document.ModelName,
				"enabled":    document.Enabled,
				"is_default": document.IsDefault,
				"updated_at": document.UpdatedAt,
			},
			"$setOnInsert": bson.M{
				"_id":        document.ID,
				"created_at": document.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (m *Store) SaveMessage(ctx context.Context, message repository.MessageRecord) error {
	_, err := m.template.InsertOne(ctx, messagesCollection, mongomapper.MessageDocumentFromRecord(message))
	return err
}

func (m *Store) SaveAsset(ctx context.Context, asset repository.AssetRecord) error {
	_, err := m.template.InsertOne(ctx, assetsCollection, mongomapper.AssetDocumentFromRecord(asset))
	return err
}

func (m *Store) ClearMessages(ctx context.Context, sessionID string) error {
	_, err := m.template.DeleteMany(ctx, messagesCollection, bson.M{"session_id": sessionID})
	return err
}

func (m *Store) ListSessions(ctx context.Context) ([]repository.SessionRecord, error) {
	return m.ListSessionsWithDeleted(ctx, false)
}

func (m *Store) ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]repository.SessionRecord, error) {
	filter := bson.M{"$or": bson.A{
		bson.M{"deleted": bson.M{"$exists": false}},
		bson.M{"deleted": false},
	}}
	if includeDeleted {
		filter = bson.M{"deleted": true}
	}

	var documents []po.SessionDocument
	err := m.template.FindAll(
		ctx,
		sessionsCollection,
		filter,
		&documents,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	sessions := make([]repository.SessionRecord, 0, len(documents))
	for _, document := range documents {
		sessions = append(sessions, mongomapper.SessionRecordFromDocument(document))
	}
	return sessions, nil
}

func (m *Store) ListConfigs(ctx context.Context) ([]domainmodel.Config, error) {
	var documents []po.ModelConfigDocument
	err := m.template.FindAll(
		ctx,
		modelConfigsCollection,
		bson.M{},
		&documents,
		options.Find().SetSort(bson.D{
			{Key: "is_default", Value: -1},
			{Key: "updated_at", Value: -1},
		}),
	)
	if err != nil {
		return nil, err
	}
	models := make([]domainmodel.Config, 0, len(documents))
	for _, document := range documents {
		models = append(models, mongomapper.ModelConfigDomainFromDocument(document))
	}
	return models, nil
}

func (m *Store) ListMessages(ctx context.Context, sessionID string) ([]repository.MessageRecord, error) {
	var documents []po.MessageDocument
	err := m.template.FindAll(
		ctx,
		messagesCollection,
		bson.M{"session_id": sessionID},
		&documents,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	messages := make([]repository.MessageRecord, 0, len(documents))
	for _, document := range documents {
		messages = append(messages, mongomapper.MessageRecordFromDocument(document))
	}
	return messages, nil
}

func (m *Store) GetMessageHistoryMeta(ctx context.Context, sessionID string) (repository.MessageHistoryMeta, error) {
	filter := bson.M{"session_id": sessionID}
	count, err := m.template.Count(ctx, messagesCollection, filter)
	if err != nil {
		return repository.MessageHistoryMeta{}, repositoryError(err)
	}

	meta := repository.MessageHistoryMeta{
		SessionID:      sessionID,
		MessageCount:   count,
		HistoryVersion: count,
	}
	if count == 0 {
		return meta, nil
	}

	var last po.MessageDocument
	err = m.template.FindOne(
		ctx,
		messagesCollection,
		filter,
		&last,
		options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}}),
	)
	if err != nil {
		return repository.MessageHistoryMeta{}, err
	}

	meta.LastMessageID = last.ID
	meta.LastMessageCreatedAt = &last.CreatedAt
	return meta, nil
}

func repositoryError(err error) error {
	if errors.Is(err, mongotemplate.ErrNotFound) {
		return repository.ErrNotFound
	}
	return err
}

func (m *Store) ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]repository.MessageRecord, bool, error) {
	messages, err := m.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > 300 {
		limit = 100
	}
	if afterMessageID == "" {
		if len(messages) <= limit {
			return messages, false, nil
		}
		return messages[:limit], false, nil
	}

	start := -1
	for index, message := range messages {
		if message.ID == afterMessageID {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return nil, true, nil
	}
	if start >= len(messages) {
		return nil, false, nil
	}

	end := start + limit
	if end > len(messages) {
		end = len(messages)
	}
	return messages[start:end], false, nil
}

func (m *Store) ListAssets(ctx context.Context, sessionID string, limit int) ([]repository.AssetRecord, error) {
	filter := bson.M{
		"$or": bson.A{
			bson.M{"deleted": bson.M{"$exists": false}},
			bson.M{"deleted": false},
		},
	}
	if sessionID != "" {
		filter["session_id"] = sessionID
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	var documents []po.AssetDocument
	err := m.template.FindAll(
		ctx,
		assetsCollection,
		filter,
		&documents,
		options.Find().
			SetSort(bson.D{{Key: "created_at", Value: -1}}).
			SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	assets := make([]repository.AssetRecord, 0, len(documents))
	for _, document := range documents {
		assets = append(assets, mongomapper.AssetRecordFromDocument(document))
	}
	return assets, nil
}
