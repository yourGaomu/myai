package mongoDb

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/store/data"
)

const (
	sessionsCollection     = "sessions"
	messagesCollection     = "messages"
	modelConfigsCollection = "model_configs"
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
	err := m.db.Collection(sessionsCollection).FindOne(ctx, bson.M{"_id": sessionID}).Decode(&session)
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
				"created_at": session.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
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

func (m *MongoStore) ClearMessages(ctx context.Context, sessionID string) error {
	if err := m.verifyDB(); err != nil {
		return err
	}

	_, err := m.db.Collection(messagesCollection).DeleteMany(ctx, bson.M{"session_id": sessionID})
	return err
}

func (m *MongoStore) ListSessions(ctx context.Context) ([]data.SessionRecord, error) {
	if err := m.verifyDB(); err != nil {
		return nil, err
	}

	cursor, err := m.db.Collection(sessionsCollection).Find(
		ctx,
		bson.M{},
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

func (m *MongoStore) verifyDB() error {
	if m.db == nil {
		return errors.New("mongo database is nil")
	}
	return nil
}
