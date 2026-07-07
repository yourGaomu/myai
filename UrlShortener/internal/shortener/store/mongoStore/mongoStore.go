package mongoStore

import (
	"context"
	"errors"
	"time"

	"myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/store"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoStore struct {
	client     *mongo.Client
	collection *mongo.Collection
}

func NewMongoStore(ctx context.Context, uri, database, collection string) (*MongoStore, error) {
	if uri == "" {
		return nil, errors.New("mongo uri is required")
	}
	if database == "" {
		return nil, errors.New("mongo database is required")
	}
	if collection == "" {
		return nil, errors.New("mongo collection is required")
	}

	client, err := mongo.Connect(options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(20).
		SetTimeout(10 * time.Second),
	)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}

	mongoCollection := client.Database(database).Collection(collection)
	if err := ensureIndexes(ctx, mongoCollection); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}

	return &MongoStore{
		client:     client,
		collection: mongoCollection,
	}, nil
}

func ensureIndexes(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (s *MongoStore) Create(ctx context.Context, link shortener.Link) error {
	if s.collection == nil {
		return errors.New("mongo collection is nil")
	}

	_, err := s.collection.InsertOne(ctx, link)
	if mongo.IsDuplicateKeyError(err) {
		return store.ErrCodeExists
	}
	return err
}

func (s *MongoStore) Get(ctx context.Context, code string) (shortener.Link, error) {
	if s.collection == nil {
		return shortener.Link{}, errors.New("mongo collection is nil")
	}

	var link shortener.Link
	err := s.collection.FindOne(ctx, activeCodeFilter(code)).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return shortener.Link{}, store.ErrLinkNotFound
	}
	if err != nil {
		return shortener.Link{}, err
	}
	return link, nil
}

func (s *MongoStore) IncrementVisits(ctx context.Context, code string) (shortener.Link, error) {
	if s.collection == nil {
		return shortener.Link{}, errors.New("mongo collection is nil")
	}

	var link shortener.Link
	err := s.collection.FindOneAndUpdate(
		ctx,
		activeCodeFilter(code),
		bson.M{"$inc": bson.M{"visits": int64(1)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return shortener.Link{}, store.ErrLinkNotFound
	}
	if err != nil {
		return shortener.Link{}, err
	}
	return link, nil
}

func (s *MongoStore) IncrementVisitsBy(ctx context.Context, code string, delta int64) error {
	if s.collection == nil {
		return errors.New("mongo collection is nil")
	}
	if delta <= 0 {
		return nil
	}

	result, err := s.collection.UpdateOne(
		ctx,
		activeCodeFilter(code),
		bson.M{
			"$inc": bson.M{"visits": delta},
			"$set": bson.M{"updated_at": time.Now()},
		},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return store.ErrLinkNotFound
	}
	return nil
}

func (s *MongoStore) Close(ctx context.Context) error {
	if s.client == nil {
		return nil
	}
	return s.client.Disconnect(ctx)
}

func (s *MongoStore) List(ctx context.Context) ([]shortener.Link, error) {
	if s.collection == nil {
		return nil, errors.New("mongo collection is nil")
	}
	find, err := s.collection.Find(ctx, bson.M{"is_deleted": bson.M{"$ne": true}})
	if err != nil {
		return nil, err
	}
	defer find.Close(ctx)

	var links []shortener.Link
	if err := find.All(ctx, &links); err != nil {
		return nil, err
	}
	return links, nil
}

func (s *MongoStore) Delete(ctx context.Context, code string) (shortener.Link, error) {
	if s.collection == nil {
		return shortener.Link{}, errors.New("mongo collection is nil")
	}

	selector := activeCodeFilter(code)
	var link shortener.Link
	err := s.collection.FindOneAndUpdate(
		ctx,
		selector,
		bson.M{"$set": bson.M{"updated_at": time.Now(), "is_deleted": true}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return shortener.Link{}, store.ErrLinkNotFound
	}
	if err != nil {
		return shortener.Link{}, err
	}
	return link, nil
}

func activeCodeFilter(code string) bson.M {
	return bson.M{
		"code":       code,
		"is_deleted": bson.M{"$ne": true},
	}
}
