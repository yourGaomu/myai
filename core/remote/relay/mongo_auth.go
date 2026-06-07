package relay

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const authorizationsCollection = "remote_authorizations"

type MongoAuthStore struct {
	db *mongo.Database
}

func NewMongoAuthStore(client *mongo.Client, database string) *MongoAuthStore {
	if client == nil || database == "" {
		return &MongoAuthStore{}
	}
	return &MongoAuthStore{db: client.Database(database)}
}

func (s *MongoAuthStore) SaveAuthorization(ctx context.Context, authorization ClientAuthorization) error {
	if err := s.verifyDB(); err != nil {
		return err
	}

	_, err := s.db.Collection(authorizationsCollection).UpdateOne(
		ctx,
		bson.M{"_id": authorization.ID},
		bson.M{
			"$set": bson.M{
				"user_id":      authorization.UserID,
				"device_id":    authorization.DeviceID,
				"client_name":  authorization.ClientName,
				"remote_addr":  authorization.RemoteAddr,
				"last_seen_at": authorization.LastSeenAt,
				"expires_at":   authorization.ExpiresAt,
				"revoked_at":   authorization.RevokedAt,
			},
			"$setOnInsert": bson.M{
				"_id":        authorization.ID,
				"created_at": authorization.CreatedAt,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (s *MongoAuthStore) GetAuthorization(ctx context.Context, id string) (ClientAuthorization, error) {
	if err := s.verifyDB(); err != nil {
		return ClientAuthorization{}, err
	}

	var authorization ClientAuthorization
	err := s.db.Collection(authorizationsCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&authorization)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ClientAuthorization{}, errAuthorizationNotFound
	}
	return authorization, err
}

func (s *MongoAuthStore) TouchAuthorization(ctx context.Context, id string, lastSeenAt time.Time) error {
	if err := s.verifyDB(); err != nil {
		return err
	}

	result, err := s.db.Collection(authorizationsCollection).UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"last_seen_at": lastSeenAt}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errAuthorizationNotFound
	}
	return nil
}

func (s *MongoAuthStore) RevokeAuthorization(ctx context.Context, id string, revokedAt time.Time) error {
	if err := s.verifyDB(); err != nil {
		return err
	}

	result, err := s.db.Collection(authorizationsCollection).UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"revoked_at": revokedAt}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errAuthorizationNotFound
	}
	return nil
}

func (s *MongoAuthStore) ListAuthorizations(ctx context.Context, userID string, deviceID string) ([]ClientAuthorization, error) {
	if err := s.verifyDB(); err != nil {
		return nil, err
	}

	cursor, err := s.db.Collection(authorizationsCollection).Find(
		ctx,
		bson.M{
			"user_id":   userID,
			"device_id": deviceID,
			"$or": bson.A{
				bson.M{"revoked_at": bson.M{"$exists": false}},
				bson.M{"revoked_at": nil},
			},
			"expires_at": bson.M{"$gt": time.Now()},
		},
		options.Find().SetSort(bson.D{{Key: "last_seen_at", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var authorizations []ClientAuthorization
	if err := cursor.All(ctx, &authorizations); err != nil {
		return nil, err
	}
	return authorizations, nil
}

func (s *MongoAuthStore) verifyDB() error {
	if s.db == nil {
		return errors.New("mongo database is nil")
	}
	return nil
}
