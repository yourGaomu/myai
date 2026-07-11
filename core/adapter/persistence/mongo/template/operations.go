package template

import (
	"context"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Operations interface {
	FindOne(ctx context.Context, collection string, filter any, out any, opts ...options.Lister[options.FindOneOptions]) error
	FindAll(ctx context.Context, collection string, filter any, out any, opts ...options.Lister[options.FindOptions]) error
	UpdateOne(ctx context.Context, collection string, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*gomongo.UpdateResult, error)
	InsertOne(ctx context.Context, collection string, document any) (*gomongo.InsertOneResult, error)
	DeleteMany(ctx context.Context, collection string, filter any) (*gomongo.DeleteResult, error)
	Count(ctx context.Context, collection string, filter any) (int64, error)
}
