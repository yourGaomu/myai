package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	authorizationmapper "myai/core/adapter/persistence/mongo/authorization/mapper"
	"myai/core/adapter/persistence/mongo/authorization/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainauthorization "myai/core/domain/authorization"
	authorizationport "myai/core/port/authorization"
)

const collection = "remote_authorizations"

type Store struct {
	operations mongotemplate.Operations
	now        func() time.Time
}

func New(client *gomongo.Client, database string) *Store {
	if client == nil || database == "" {
		return NewWithOperations(mongotemplate.New(nil))
	}
	return NewWithOperations(mongotemplate.New(client.Database(database)))
}

func NewWithOperations(operations mongotemplate.Operations) *Store {
	if operations == nil {
		operations = mongotemplate.New(nil)
	}
	return &Store{operations: operations, now: time.Now}
}

func (s *Store) Save(ctx context.Context, authorization domainauthorization.ClientAuthorization) error {
	document := authorizationmapper.DocumentFromDomain(authorization)
	_, err := s.operations.UpdateOne(
		ctx,
		collection,
		bson.M{"_id": document.ID},
		bson.M{
			"$set": bson.M{
				"user_id":      document.UserID,
				"device_id":    document.DeviceID,
				"client_name":  document.ClientName,
				"remote_addr":  document.RemoteAddr,
				"last_seen_at": document.LastSeenAt,
				"expires_at":   document.ExpiresAt,
				"revoked_at":   document.RevokedAt,
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

func (s *Store) Get(ctx context.Context, id string) (domainauthorization.ClientAuthorization, error) {
	var document po.Document
	err := s.operations.FindOne(ctx, collection, bson.M{"_id": id}, &document)
	if err != nil {
		return domainauthorization.ClientAuthorization{}, portError(err)
	}
	return authorizationmapper.DomainFromDocument(document), nil
}

func (s *Store) Touch(ctx context.Context, id string, lastSeenAt time.Time) error {
	return s.updateExisting(ctx, id, bson.M{"last_seen_at": lastSeenAt})
}

func (s *Store) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	return s.updateExisting(ctx, id, bson.M{"revoked_at": revokedAt})
}

func (s *Store) ListActive(ctx context.Context, userID string, deviceID string) ([]domainauthorization.ClientAuthorization, error) {
	var documents []po.Document
	err := s.operations.FindAll(
		ctx,
		collection,
		bson.M{
			"user_id":   userID,
			"device_id": deviceID,
			"$or": bson.A{
				bson.M{"revoked_at": bson.M{"$exists": false}},
				bson.M{"revoked_at": nil},
			},
			"expires_at": bson.M{"$gt": s.currentTime()},
		},
		&documents,
		options.Find().SetSort(bson.D{{Key: "last_seen_at", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}

	authorizations := make([]domainauthorization.ClientAuthorization, 0, len(documents))
	for _, document := range documents {
		authorizations = append(authorizations, authorizationmapper.DomainFromDocument(document))
	}
	return authorizations, nil
}

func (s *Store) updateExisting(ctx context.Context, id string, fields bson.M) error {
	result, err := s.operations.UpdateOne(
		ctx,
		collection,
		bson.M{"_id": id},
		bson.M{"$set": fields},
	)
	if err != nil {
		return err
	}
	if result == nil || result.MatchedCount == 0 {
		return authorizationport.ErrNotFound
	}
	return nil
}

func (s *Store) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func portError(err error) error {
	if errors.Is(err, mongotemplate.ErrNotFound) {
		return authorizationport.ErrNotFound
	}
	return err
}
