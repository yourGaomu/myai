package template

import (
	"context"
	"errors"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Template struct {
	database *gomongo.Database
}

func New(database *gomongo.Database) *Template {
	return &Template{database: database}
}

func (t *Template) FindOne(ctx context.Context, collection string, filter any, out any, opts ...options.Lister[options.FindOneOptions]) error {
	target, err := t.collection(collection)
	if err != nil {
		return err
	}
	if out == nil {
		return errors.New("mongo find one output is nil")
	}
	err = target.FindOne(ctx, filter, opts...).Decode(out)
	return translateError(err)
}

func (t *Template) FindAll(ctx context.Context, collection string, filter any, out any, opts ...options.Lister[options.FindOptions]) error {
	target, err := t.collection(collection)
	if err != nil {
		return err
	}
	if out == nil {
		return errors.New("mongo find output is nil")
	}

	cursor, err := target.Find(ctx, filter, opts...)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	return cursor.All(ctx, out)
}

func (t *Template) UpdateOne(ctx context.Context, collection string, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error) {
	target, err := t.collection(collection)
	if err != nil {
		return nil, err
	}
	return target.UpdateOne(ctx, filter, update, opts...)
}

func (t *Template) InsertOne(ctx context.Context, collection string, document any) (*gomongo.InsertOneResult, error) {
	target, err := t.collection(collection)
	if err != nil {
		return nil, err
	}
	return target.InsertOne(ctx, document)
}

func (t *Template) DeleteMany(ctx context.Context, collection string, filter any) (*gomongo.DeleteResult, error) {
	target, err := t.collection(collection)
	if err != nil {
		return nil, err
	}
	return target.DeleteMany(ctx, filter)
}

func (t *Template) Count(ctx context.Context, collection string, filter any) (int64, error) {
	target, err := t.collection(collection)
	if err != nil {
		return 0, err
	}
	return target.CountDocuments(ctx, filter)
}

func (t *Template) collection(name string) (*gomongo.Collection, error) {
	if t == nil || t.database == nil {
		return nil, errors.New("mongo database is nil")
	}
	if name == "" {
		return nil, errors.New("mongo collection is empty")
	}
	return t.database.Collection(name), nil
}

func translateError(err error) error {
	if errors.Is(err, gomongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}
