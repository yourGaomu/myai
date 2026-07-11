package mongo

import (
	mongotemplate "myai/core/adapter/persistence/mongo/template"

	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
)

type Operations = mongotemplate.Operations
type Template = mongotemplate.Template

func NewTemplate(database *gomongo.Database) *Template {
	return mongotemplate.New(database)
}
