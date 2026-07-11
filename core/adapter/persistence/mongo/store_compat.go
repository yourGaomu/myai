package mongo

import (
	mongorepository "myai/core/adapter/persistence/mongo/repository"
	mongotemplate "myai/core/adapter/persistence/mongo/template"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
)

type Store = mongorepository.Store

func New(client *gomongo.Client, database string) *Store {
	return mongorepository.New(client, database)
}

func NewWithTemplate(template mongotemplate.Operations) *Store {
	return mongorepository.NewWithTemplate(template)
}
