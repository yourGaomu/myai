package authorization

import (
	authorizationrepository "myai/core/adapter/persistence/mongo/authorization/repository"
	mongotemplate "myai/core/adapter/persistence/mongo/template"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
)

type Store = authorizationrepository.Store

func New(client *gomongo.Client, database string) *Store {
	return authorizationrepository.New(client, database)
}

func NewWithOperations(operations mongotemplate.Operations) *Store {
	return authorizationrepository.NewWithOperations(operations)
}
