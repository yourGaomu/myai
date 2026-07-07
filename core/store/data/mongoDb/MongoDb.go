package mongoDb

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/store/data"
)

const (
	sessionsCollection     = "sessions"
	messagesCollection     = "messages"
	modelConfigsCollection = "model_configs"
	assetsCollection       = "assets"
)

type MongoStore struct {
	db *mongo.Database
}

func New(client *mongo.Client, database string) *MongoStore {
	if client == nil || database == "" {
		return &MongoStore{}
	}
	return &MongoStore{db: client.Database(database)}
}

func (m *MongoStore) GetSession(ctx context.Context, sessionID string) (data.SessionRecord, error) {
	if err := m.verifyDB(); err != nil {
		return data.SessionRecord{}, err
	}

	var session data.SessionRecord
	err := m.db.Collection(sessionsCollection).FindOne(ctx, bson.M{
		"_id": sessionID,
		"$or": bson.A{
			bson.M{"deleted": bson.M{"$exists": false}},
			bson.M{"deleted": false},
		},
	}).Decode(&session)
	if err != nil {
		return data.SessionRecord{}, err
	}

	return session, nil
}

func (m *MongoStore) SaveSession(ctx context.Context, session data.SessionRecord) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(sessionsCollection).UpdateOne(
		ctx,
		bson.M{"_id": session.ID},
		bson.M{
			"$set": bson.M{
				"model":              session.Model,
				"permission_mode":    session.PermissionMode,
				"context_window_k":   session.ContextWindowK,
				"summary":            session.Summary,
				"compacted_messages": session.CompactedMessages,
				"compacted_at":       session.CompactedAt,
				"title":              session.Title,
				"usage":              session.Usage,
				"last_usage":         session.LastUsage,
				"updated_at":         session.UpdatedAt,
			},
			"$setOnInsert": bson.M{
				"_id":        session.ID,
				"deleted":    false,
				"created_at": session.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (m *MongoStore) MarkSessionDeleted(ctx context.Context, sessionID string, deletedAt time.Time) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(sessionsCollection).UpdateOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{"$set": bson.M{
			"deleted":    true,
			"deleted_at": deletedAt,
			"updated_at": deletedAt,
		}},
	)
	return err
}

func (m *MongoStore) MarkSessionRestored(ctx context.Context, sessionID string, restoredAt time.Time) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(sessionsCollection).UpdateOne(
		ctx,
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

func (m *MongoStore) SaveModelConfig(ctx context.Context, model data.ModelConfig) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(modelConfigsCollection).UpdateOne(
		ctx,
		bson.M{"_id": model.ID},
		bson.M{
			"$set": bson.M{
				"name":       model.Name,
				"provider":   model.Provider,
				"base_url":   model.BaseURL,
				"api_key":    model.APIKey,
				"model_name": model.ModelName,
				"enabled":    model.Enabled,
				"is_default": model.IsDefault,
				"updated_at": model.UpdatedAt,
			},
			"$setOnInsert": bson.M{
				"_id":        model.ID,
				"created_at": model.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (m *MongoStore) SaveMessage(ctx context.Context, message data.MessageRecord) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(messagesCollection).InsertOne(ctx, message)
	return err
}

func (m *MongoStore) SaveAsset(ctx context.Context, asset data.AssetRecord) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(assetsCollection).InsertOne(ctx, asset)
	return err
}

func (m *MongoStore) ClearMessages(ctx context.Context, sessionID string) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(messagesCollection).DeleteMany(ctx, bson.M{"session_id": sessionID})
	return err
}

func (m *MongoStore) ListSessions(ctx context.Context) ([]data.SessionRecord, error) {
	return m.ListSessionsWithDeleted(ctx, false)
}

func (m *MongoStore) ListSessionsWithDeleted(ctx context.Context, includeDeleted bool) ([]data.SessionRecord, error) {
	if err := m.verifyDB(); err != nil {
		return nil, err
	}

	filter := bson.M{"$or": bson.A{
		bson.M{"deleted": bson.M{"$exists": false}},
		bson.M{"deleted": false},
	}}
	if includeDeleted {
		filter = bson.M{"deleted": true}
	}

	cursor, err := m.db.Collection(sessionsCollection).Find(
		ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []data.SessionRecord
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (m *MongoStore) ListModelConfigs(ctx context.Context) ([]data.ModelConfig, error) {
	if err := m.verifyDB(); err != nil {
		return nil, err
	}

	cursor, err := m.db.Collection(modelConfigsCollection).Find(
		ctx,
		bson.M{},
		options.Find().SetSort(bson.D{
			{Key: "is_default", Value: -1},
			{Key: "updated_at", Value: -1},
		}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var models []data.ModelConfig
	if err := cursor.All(ctx, &models); err != nil {
		return nil, err
	}

	return models, nil
}

func (m *MongoStore) ListMessages(ctx context.Context, sessionID string) ([]data.MessageRecord, error) {
	if err := m.verifyDB(); err != nil {
		return nil, err
	}

	cursor, err := m.db.Collection(messagesCollection).Find(
		ctx,
		bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []data.MessageRecord
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

func (m *MongoStore) GetMessageHistoryMeta(ctx context.Context, sessionID string) (data.MessageHistoryMeta, error) {
	if err := m.verifyDB(); err != nil {
		return data.MessageHistoryMeta{}, err
	}

	filter := bson.M{"session_id": sessionID}
	count, err := m.db.Collection(messagesCollection).CountDocuments(ctx, filter)
	if err != nil {
		return data.MessageHistoryMeta{}, err
	}

	meta := data.MessageHistoryMeta{
		SessionID:      sessionID,
		MessageCount:   count,
		HistoryVersion: count,
	}
	if count == 0 {
		return meta, nil
	}

	var last data.MessageRecord
	err = m.db.Collection(messagesCollection).FindOne(
		ctx,
		filter,
		options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}}),
	).Decode(&last)
	if err != nil {
		return data.MessageHistoryMeta{}, err
	}

	meta.LastMessageID = last.ID
	meta.LastMessageCreatedAt = &last.CreatedAt
	return meta, nil
}

func (m *MongoStore) ListMessagesAfter(ctx context.Context, sessionID string, afterMessageID string, limit int) ([]data.MessageRecord, bool, error) {
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

func (m *MongoStore) ListAssets(ctx context.Context, sessionID string, limit int) ([]data.AssetRecord, error) {
	if err := m.verifyDB(); err != nil {
		return nil, err
	}

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

	cursor, err := m.db.Collection(assetsCollection).Find(
		ctx,
		filter,
		options.Find().
			SetSort(bson.D{{Key: "created_at", Value: -1}}).
			SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var assets []data.AssetRecord
	if err := cursor.All(ctx, &assets); err != nil {
		return nil, err
	}

	return assets, nil
}

func (m *MongoStore) verifyDB() error {
	if m.db == nil {
		return errors.New("mongo database is nil")
	}
	return nil
}
