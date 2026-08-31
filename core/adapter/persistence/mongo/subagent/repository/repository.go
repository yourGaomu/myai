package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/subagent/mapper"
	"myai/core/adapter/persistence/mongo/subagent/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainsubagent "myai/core/domain/subagent"
	subagentport "myai/core/port/subagent"
)

const (
	definitionsCollection = "subagent_definitions"
	tasksCollection       = "subagent_tasks"
	runsCollection        = "subagent_runs"
	eventsCollection      = "subagent_task_events"
)

type Repository struct {
	template mongotemplate.Operations
	database *gomongo.Database
}

var _ subagentport.DefinitionRepository = (*Repository)(nil)
var _ subagentport.TaskRepository = (*Repository)(nil)
var _ subagentport.RunRepository = (*Repository)(nil)
var _ subagentport.TaskRunRepository = (*Repository)(nil)
var _ subagentport.InterruptedTaskRepository = (*Repository)(nil)
var _ subagentport.TaskEventRepository = (*Repository)(nil)
var _ subagentport.TaskEventTailRepository = (*Repository)(nil)

func New(client *gomongo.Client, database string) *Repository {
	if client == nil || strings.TrimSpace(database) == "" {
		return &Repository{template: mongotemplate.New(nil)}
	}
	target := client.Database(database)
	return &Repository{template: mongotemplate.New(target), database: target}
}

func (repository *Repository) SaveDefinition(ctx context.Context, definition domainsubagent.Definition) error {
	document := mapper.DefinitionDocumentFromDomain(definition)
	update, err := replacementUpdate(document, "description", "model_id", "allowed_tools", "deleted_at")
	if err != nil {
		return err
	}
	_, err = repository.template.UpdateOne(ctx, definitionsCollection, bson.M{"_id": document.ID}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (repository *Repository) GetDefinition(ctx context.Context, definitionID string) (domainsubagent.Definition, error) {
	var document po.DefinitionDocument
	err := repository.template.FindOne(ctx, definitionsCollection, bson.M{"_id": strings.TrimSpace(definitionID)}, &document)
	if err != nil {
		return domainsubagent.Definition{}, translateError(err)
	}
	return mapper.DefinitionDomainFromDocument(document), nil
}

func (repository *Repository) ListDefinitions(ctx context.Context, includeDeleted bool) ([]domainsubagent.Definition, error) {
	filter := bson.M{"$or": bson.A{bson.M{"deleted": bson.M{"$exists": false}}, bson.M{"deleted": false}}}
	if includeDeleted {
		filter = bson.M{}
	}
	var documents []po.DefinitionDocument
	err := repository.template.FindAll(ctx, definitionsCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	items := make([]domainsubagent.Definition, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.DefinitionDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) DeleteDefinition(ctx context.Context, definitionID string) error {
	now := time.Now().UTC()
	result, err := repository.template.UpdateOne(ctx, definitionsCollection, bson.M{"_id": strings.TrimSpace(definitionID), "deleted": bson.M{"$ne": true}}, bson.M{
		"$set": bson.M{"deleted": true, "deleted_at": now, "updated_at": now},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrNotFound
	}
	return nil
}

func (repository *Repository) SaveTask(ctx context.Context, task domainsubagent.Task) error {
	document := mapper.TaskDocumentFromDomain(task)
	update, err := replacementUpdate(
		document,
		"created_request_id", "change_set", "result", "reasoning", "error_message", "started_at", "completed_at",
	)
	if err != nil {
		return err
	}
	_, err = repository.template.UpdateOne(ctx, tasksCollection, bson.M{"_id": document.ID}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (repository *Repository) GetTask(ctx context.Context, taskID string) (domainsubagent.Task, error) {
	var document po.TaskDocument
	err := repository.template.FindOne(ctx, tasksCollection, bson.M{"_id": strings.TrimSpace(taskID)}, &document)
	if err != nil {
		return domainsubagent.Task{}, translateError(err)
	}
	return mapper.TaskDomainFromDocument(document), nil
}

func (repository *Repository) ListTasks(ctx context.Context, parentSessionID string, limit int) ([]domainsubagent.Task, error) {
	filter := bson.M{}
	if parentSessionID = strings.TrimSpace(parentSessionID); parentSessionID != "" {
		filter["parent_session_id"] = parentSessionID
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var documents []po.TaskDocument
	err := repository.template.FindAll(ctx, tasksCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	items := make([]domainsubagent.Task, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.TaskDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) ListNonTerminalTasks(ctx context.Context) ([]domainsubagent.Task, error) {
	filter := bson.M{"status": bson.M{"$nin": bson.A{
		string(domainsubagent.TaskStatusSucceeded),
		string(domainsubagent.TaskStatusFailed),
		string(domainsubagent.TaskStatusCanceled),
	}}}
	var documents []po.TaskDocument
	if err := repository.template.FindAll(ctx, tasksCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})); err != nil {
		return nil, err
	}
	items := make([]domainsubagent.Task, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.TaskDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) SaveTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		if err := repository.SaveTask(transactionContext, task); err != nil {
			return nil, err
		}
		if err := repository.SaveRun(transactionContext, run); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

func (repository *Repository) SaveRun(ctx context.Context, run domainsubagent.Run) error {
	document := mapper.RunDocumentFromDomain(run)
	update, err := replacementUpdate(document, "result", "error_message", "started_at", "completed_at")
	if err != nil {
		return err
	}
	_, err = repository.template.UpdateOne(ctx, runsCollection, bson.M{"_id": document.ID}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (repository *Repository) ListRuns(ctx context.Context, taskID string) ([]domainsubagent.Run, error) {
	var documents []po.RunDocument
	err := repository.template.FindAll(ctx, runsCollection, bson.M{"task_id": strings.TrimSpace(taskID)}, &documents, options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}}))
	if err != nil {
		return nil, err
	}
	items := make([]domainsubagent.Run, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.RunDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) SaveTaskEvent(ctx context.Context, event subagentport.TaskEvent) error {
	document := mapper.TaskEventDocumentFromDomain(event)
	if document.Sequence == 0 {
		return errors.New("subagent task event sequence is required")
	}
	update, err := replacementUpdate(document)
	if err != nil {
		return err
	}
	_, err = repository.template.UpdateOne(ctx, eventsCollection, bson.M{"_id": fmt.Sprintf("%020d", document.Sequence)}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (repository *Repository) ListTaskEvents(ctx context.Context, parentSessionID string, afterSequence uint64, limit int) ([]subagentport.TaskEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 512
	}
	filter := bson.M{"sequence": bson.M{"$gt": afterSequence}}
	if parentSessionID = strings.TrimSpace(parentSessionID); parentSessionID != "" {
		filter["task.parent_session_id"] = parentSessionID
	}
	var documents []po.TaskEventDocument
	if err := repository.template.FindAll(ctx, eventsCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]subagentport.TaskEvent, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.TaskEventDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) LatestTaskEventSequence(ctx context.Context) (uint64, error) {
	var documents []po.TaskEventDocument
	if err := repository.template.FindAll(ctx, eventsCollection, bson.M{}, &documents, options.Find().SetSort(bson.D{{Key: "sequence", Value: -1}}).SetLimit(1)); err != nil {
		return 0, err
	}
	if len(documents) == 0 {
		return 0, nil
	}
	return documents[0].Sequence, nil
}

func translateError(err error) error {
	if errors.Is(err, mongotemplate.ErrNotFound) {
		return subagentport.ErrNotFound
	}
	return err
}

func replacementUpdate(document any, optionalFields ...string) (bson.M, error) {
	encoded, err := bson.Marshal(document)
	if err != nil {
		return nil, err
	}
	setValues := bson.M{}
	if err := bson.Unmarshal(encoded, &setValues); err != nil {
		return nil, err
	}
	delete(setValues, "_id")

	update := bson.M{"$set": setValues}
	unsetValues := bson.M{}
	for _, field := range optionalFields {
		if _, exists := setValues[field]; !exists {
			unsetValues[field] = ""
		}
	}
	if len(unsetValues) > 0 {
		update["$unset"] = unsetValues
	}
	return update, nil
}
