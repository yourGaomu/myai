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
	definitionsCollection   = "subagent_definitions"
	tasksCollection         = "subagent_tasks"
	runsCollection          = "subagent_runs"
	eventsCollection        = "subagent_task_events"
	eventCountersCollection = "subagent_event_counters"
	messagesCollection      = "subagent_agent_messages"
	leasesCollection        = "subagent_execution_leases"
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
var _ subagentport.TaskEventSequenceRepository = (*Repository)(nil)
var _ subagentport.TaskEventSequenceInitializer = (*Repository)(nil)
var _ subagentport.AgentMessageRepository = (*Repository)(nil)
var _ subagentport.ParentCompletionMessageRepository = (*Repository)(nil)
var _ subagentport.ChildAgentMessageRepository = (*Repository)(nil)
var _ subagentport.AgentExecutionLeaseStore = (*Repository)(nil)
var _ subagentport.LeaseGuardedTaskRunRepository = (*Repository)(nil)
var _ subagentport.TaskCancellationRepository = (*Repository)(nil)
var _ subagentport.TaskVersionRepository = (*Repository)(nil)
var _ subagentport.LeaseGuardedTaskRepository = (*Repository)(nil)

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
		"created_request_id", "change_set", "result", "reasoning", "error_message", "mailbox", "started_at", "completed_at",
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

func (repository *Repository) SaveOwnedTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run, ownerID string, ttl time.Duration) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	if task.ID == "" || run.ID == "" || run.TaskID != task.ID || strings.TrimSpace(ownerID) == "" || ttl <= 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		now := time.Now().UTC()
		result, err := repository.template.UpdateOne(transactionContext, leasesCollection, bson.M{
			"_id": task.ID, "run_id": run.ID, "owner_id": ownerID,
			"expires_at": bson.M{"$gt": now},
		}, bson.M{"$set": bson.M{"expires_at": now.Add(ttl), "updated_at": now}})
		if err != nil {
			return nil, err
		}
		if result.MatchedCount == 0 {
			return nil, subagentport.ErrExecutionLeaseNotAcquired
		}
		current, loadErr := repository.GetTask(transactionContext, task.ID)
		if loadErr != nil && !errors.Is(loadErr, subagentport.ErrNotFound) {
			return nil, loadErr
		}
		if loadErr == nil {
			task, err = subagentport.ReconcileOwnedTaskTransition(current, task, run.ID)
			if err != nil {
				return nil, err
			}
		}
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

func (repository *Repository) SaveCanceledTaskAndRun(ctx context.Context, task domainsubagent.Task, run domainsubagent.Run) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	if task.ID == "" || run.ID == "" || run.TaskID != task.ID {
		return subagentport.ErrTaskStateConflict
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		current, err := repository.GetTask(transactionContext, task.ID)
		if err != nil {
			return nil, err
		}
		if current.CurrentRunID != run.ID || (current.Terminal() && current.Status != domainsubagent.TaskStatusCanceled) {
			return nil, subagentport.ErrTaskStateConflict
		}
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

func (repository *Repository) SaveTaskIfVersion(ctx context.Context, task domainsubagent.Task, expectedUpdatedAt time.Time) error {
	document := mapper.TaskDocumentFromDomain(task)
	update, err := replacementUpdate(document, "created_request_id", "change_set", "result", "reasoning", "error_message", "mailbox", "started_at", "completed_at")
	if err != nil {
		return err
	}
	result, err := repository.template.UpdateOne(ctx, tasksCollection, bson.M{"_id": document.ID, "updated_at": expectedUpdatedAt}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrTaskStateConflict
	}
	return nil
}

func (repository *Repository) SaveOwnedTask(ctx context.Context, task domainsubagent.Task, leaseRunID, expectedCurrentRunID, ownerID string, expectedUpdatedAt time.Time, ttl time.Duration) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	if task.ID == "" || strings.TrimSpace(leaseRunID) == "" || strings.TrimSpace(ownerID) == "" || ttl <= 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		now := time.Now().UTC()
		leaseResult, err := repository.template.UpdateOne(transactionContext, leasesCollection, bson.M{
			"_id": task.ID, "run_id": leaseRunID, "owner_id": ownerID, "expires_at": bson.M{"$gt": now},
		}, bson.M{"$set": bson.M{"expires_at": now.Add(ttl), "updated_at": now}})
		if err != nil {
			return nil, err
		}
		if leaseResult.MatchedCount == 0 {
			return nil, subagentport.ErrExecutionLeaseNotAcquired
		}
		current, err := repository.GetTask(transactionContext, task.ID)
		if err != nil {
			return nil, err
		}
		if current.CurrentRunID != expectedCurrentRunID || !current.UpdatedAt.Equal(expectedUpdatedAt) {
			return nil, subagentport.ErrTaskStateConflict
		}
		return nil, repository.SaveTask(transactionContext, task)
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

func (repository *Repository) NextTaskEventSequence(ctx context.Context) (uint64, error) {
	if repository == nil || repository.database == nil {
		return 0, errors.New("mongo subagent database is nil")
	}
	var counter po.TaskEventSequenceDocument
	err := repository.database.Collection(eventCountersCollection).FindOneAndUpdate(
		ctx,
		bson.M{"_id": eventsCollection},
		bson.M{"$inc": bson.M{"value": int64(1)}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&counter)
	if err != nil {
		return 0, err
	}
	if counter.Value <= 0 {
		return 0, errors.New("mongo task event sequence is invalid")
	}
	return uint64(counter.Value), nil
}

func (repository *Repository) EnsureTaskEventSequence(ctx context.Context, atLeast uint64) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	_, err := repository.database.Collection(eventCountersCollection).UpdateOne(
		ctx,
		bson.M{"_id": eventsCollection},
		bson.M{"$max": bson.M{"value": int64(atLeast)}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func (repository *Repository) EnqueueChildAgentMessage(ctx context.Context, taskID string, message domainsubagent.AgentMessage) (domainsubagent.Task, error) {
	if repository == nil || repository.database == nil {
		return domainsubagent.Task{}, errors.New("mongo subagent database is nil")
	}
	if err := message.Validate(); err != nil {
		return domainsubagent.Task{}, err
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return domainsubagent.Task{}, err
	}
	defer session.EndSession(context.Background())
	var saved domainsubagent.Task
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		task, err := repository.GetTask(transactionContext, taskID)
		if err != nil {
			return nil, err
		}
		existing, err := repository.GetAgentMessage(transactionContext, message.ID)
		if err == nil {
			if !existing.SameRequest(message) {
				return nil, errors.New("agent message id was already used with different request")
			}
			saved = task
			return nil, nil
		}
		if !errors.Is(err, subagentport.ErrNotFound) {
			return nil, err
		}
		for _, queued := range task.Mailbox {
			if queued.ID == message.ID {
				return nil, errors.New("agent message id already exists in task mailbox")
			}
		}
		if err := task.EnqueueMessage(domainsubagent.Message{ID: message.ID, Content: message.Content, Trigger: message.Trigger, CreatedAt: message.CreatedAt}); err != nil {
			return nil, err
		}
		if err := repository.SaveTask(transactionContext, task); err != nil {
			return nil, err
		}
		if err := repository.SaveAgentMessage(transactionContext, message); err != nil {
			return nil, err
		}
		saved = task
		return nil, nil
	})
	return saved, err
}

func (repository *Repository) ClaimChildMailboxMessage(ctx context.Context, taskID string, now time.Time) (domainsubagent.Message, bool, error) {
	if repository == nil || repository.database == nil {
		return domainsubagent.Message{}, false, errors.New("mongo subagent database is nil")
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return domainsubagent.Message{}, false, err
	}
	defer session.EndSession(context.Background())
	var claimed domainsubagent.Message
	var ok bool
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		task, err := repository.GetTask(transactionContext, taskID)
		if err != nil {
			return nil, err
		}
		claimed, ok = task.ClaimMailboxMessage(now)
		if !ok {
			return nil, nil
		}
		if err := repository.SaveTask(transactionContext, task); err != nil {
			return nil, err
		}
		envelope, err := repository.GetAgentMessage(transactionContext, claimed.ID)
		if err != nil {
			return nil, err
		}
		envelope.Status = domainsubagent.AgentMessageDelivering
		envelope.DeliveryAttempts = claimed.DeliveryAttempts
		envelope.LastError = ""
		if err := repository.SaveAgentMessage(transactionContext, envelope); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return claimed, ok, err
}

func (repository *Repository) AcknowledgeChildAgentMessage(ctx context.Context, taskID, messageID string, deliveredAt time.Time) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		task, err := repository.GetTask(transactionContext, taskID)
		if err != nil {
			return nil, err
		}
		message, err := repository.GetAgentMessage(transactionContext, messageID)
		if err != nil {
			return nil, err
		}
		if message.Status == domainsubagent.AgentMessageDelivered {
			return nil, nil
		}
		if message.RecipientAgentID != task.ChildSessionID || !task.AcknowledgeMailboxMessage(messageID, deliveredAt) {
			return nil, errors.New("claimed child agent message is missing from task mailbox")
		}
		if err := repository.SaveTask(transactionContext, task); err != nil {
			return nil, err
		}
		if err := repository.MarkAgentMessageDelivered(transactionContext, messageID, deliveredAt); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

func (repository *Repository) ReleaseChildMailboxMessage(ctx context.Context, taskID, messageID, reason string, now time.Time) error {
	if repository == nil || repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		task, err := repository.GetTask(transactionContext, taskID)
		if err != nil {
			return nil, err
		}
		var deliveryErr error
		if reason != "" {
			deliveryErr = errors.New(reason)
		}
		if !task.ReleaseMailboxMessage(messageID, deliveryErr, now) {
			return nil, nil
		}
		if err := repository.SaveTask(transactionContext, task); err != nil {
			return nil, err
		}
		envelope, err := repository.GetAgentMessage(transactionContext, messageID)
		if err != nil {
			return nil, err
		}
		envelope.Status = domainsubagent.AgentMessagePending
		envelope.LastError = reason
		return nil, repository.SaveAgentMessage(transactionContext, envelope)
	})
	return err
}

func (repository *Repository) SaveAgentMessage(ctx context.Context, message domainsubagent.AgentMessage) error {
	if err := message.Validate(); err != nil {
		return err
	}
	document := mapper.AgentMessageDocumentFromDomain(message)
	update, err := replacementUpdate(document)
	if err != nil {
		return err
	}
	_, err = repository.template.UpdateOne(ctx, messagesCollection, bson.M{"_id": document.ID}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (repository *Repository) GetAgentMessage(ctx context.Context, messageID string) (domainsubagent.AgentMessage, error) {
	var document po.AgentMessageDocument
	err := repository.template.FindOne(ctx, messagesCollection, bson.M{"_id": strings.TrimSpace(messageID)}, &document)
	if err != nil {
		return domainsubagent.AgentMessage{}, translateError(err)
	}
	return mapper.AgentMessageDomainFromDocument(document), nil
}

func (repository *Repository) ListPendingAgentMessages(ctx context.Context, recipientAgentID string, limit int) ([]domainsubagent.AgentMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	filter := bson.M{"status": bson.M{"$in": bson.A{
		string(domainsubagent.AgentMessagePending), string(domainsubagent.AgentMessageDelivering),
	}}}
	if recipientAgentID = strings.TrimSpace(recipientAgentID); recipientAgentID != "" {
		filter["recipient_agent_id"] = recipientAgentID
	}
	var documents []po.AgentMessageDocument
	if err := repository.template.FindAll(ctx, messagesCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]domainsubagent.AgentMessage, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.AgentMessageDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) ListPendingParentMessages(ctx context.Context) ([]domainsubagent.AgentMessage, error) {
	filter := bson.M{"status": bson.M{"$in": bson.A{
		string(domainsubagent.AgentMessagePending), string(domainsubagent.AgentMessageDelivering),
	}}, "kind": bson.M{"$in": bson.A{
		string(domainsubagent.AgentMessageKindTaskResult), string(domainsubagent.AgentMessageKindTaskError),
	}}}
	var documents []po.AgentMessageDocument
	if err := repository.template.FindAll(ctx, messagesCollection, filter, &documents, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})); err != nil {
		return nil, err
	}
	items := make([]domainsubagent.AgentMessage, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.AgentMessageDomainFromDocument(document))
	}
	return items, nil
}

func (repository *Repository) MarkAgentMessageDelivered(ctx context.Context, messageID string, deliveredAt time.Time) error {
	result, err := repository.template.UpdateOne(ctx, messagesCollection, bson.M{"_id": strings.TrimSpace(messageID)}, bson.M{
		"$set":   bson.M{"status": string(domainsubagent.AgentMessageDelivered), "delivered_at": deliveredAt},
		"$unset": bson.M{"claim_owner_id": "", "claim_expires_at": ""},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrNotFound
	}
	return nil
}

func (repository *Repository) EnqueueParentCompletion(ctx context.Context, taskID string, message domainsubagent.AgentMessage) (domainsubagent.AgentMessage, error) {
	if repository == nil || repository.database == nil {
		return domainsubagent.AgentMessage{}, errors.New("mongo subagent database is nil")
	}
	if err := message.Validate(); err != nil {
		return domainsubagent.AgentMessage{}, err
	}
	session, err := repository.database.Client().StartSession()
	if err != nil {
		return domainsubagent.AgentMessage{}, err
	}
	defer session.EndSession(context.Background())
	var saved domainsubagent.AgentMessage
	_, err = session.WithTransaction(ctx, func(transactionContext context.Context) (any, error) {
		task, err := repository.GetTask(transactionContext, taskID)
		if err != nil {
			return nil, err
		}
		if !task.Terminal() || (message.Kind == domainsubagent.AgentMessageKindTaskResult && task.Status != domainsubagent.TaskStatusSucceeded) ||
			(message.Kind == domainsubagent.AgentMessageKindTaskError && task.Status != domainsubagent.TaskStatusFailed) {
			return nil, subagentport.ErrTaskStateConflict
		}
		existing, err := repository.GetAgentMessage(transactionContext, message.ID)
		if err == nil {
			if existing.SourceTaskID != taskID || existing.RecipientAgentID != task.ParentSessionID || existing.Kind != message.Kind || existing.Content != message.Content {
				return nil, errors.New("parent completion message conflicts with existing envelope")
			}
			saved = existing
			return nil, nil
		}
		if !errors.Is(err, subagentport.ErrNotFound) {
			return nil, err
		}
		if err := repository.SaveAgentMessage(transactionContext, message); err != nil {
			return nil, err
		}
		saved = message
		return nil, nil
	})
	return saved, err
}

func (repository *Repository) ClaimParentCompletion(ctx context.Context, messageID, ownerID string, now time.Time, ttl time.Duration) (domainsubagent.AgentMessage, error) {
	if repository == nil || repository.database == nil {
		return domainsubagent.AgentMessage{}, errors.New("mongo subagent database is nil")
	}
	if strings.TrimSpace(ownerID) == "" || ttl <= 0 {
		return domainsubagent.AgentMessage{}, errors.New("parent completion claim is invalid")
	}
	expires := now.Add(ttl)
	result, err := repository.template.UpdateOne(ctx, messagesCollection, bson.M{
		"_id": strings.TrimSpace(messageID), "$or": bson.A{
			bson.M{"status": string(domainsubagent.AgentMessagePending)},
			bson.M{"status": string(domainsubagent.AgentMessageDelivering), "claim_expires_at": bson.M{"$lte": now}},
			bson.M{"status": string(domainsubagent.AgentMessageDelivering), "claim_owner_id": ownerID},
		},
	}, bson.M{"$set": bson.M{"status": string(domainsubagent.AgentMessageDelivering), "claim_owner_id": ownerID, "claim_expires_at": expires}, "$inc": bson.M{"delivery_attempts": 1}})
	if err != nil {
		return domainsubagent.AgentMessage{}, err
	}
	if result.MatchedCount == 0 {
		return domainsubagent.AgentMessage{}, subagentport.ErrExecutionLeaseNotAcquired
	}
	return repository.GetAgentMessage(ctx, messageID)
}

func (repository *Repository) ReleaseParentCompletion(ctx context.Context, messageID, ownerID, reason string) error {
	result, err := repository.template.UpdateOne(ctx, messagesCollection, bson.M{"_id": strings.TrimSpace(messageID), "status": string(domainsubagent.AgentMessageDelivering), "claim_owner_id": ownerID}, bson.M{
		"$set":   bson.M{"status": string(domainsubagent.AgentMessagePending), "last_error": strings.TrimSpace(reason)},
		"$unset": bson.M{"claim_owner_id": "", "claim_expires_at": ""},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrTaskStateConflict
	}
	return nil
}

func (repository *Repository) CompleteParentCompletion(ctx context.Context, messageID, ownerID string, deliveredAt time.Time) error {
	result, err := repository.template.UpdateOne(ctx, messagesCollection, bson.M{
		"_id": strings.TrimSpace(messageID), "status": string(domainsubagent.AgentMessageDelivering), "claim_owner_id": ownerID,
	}, bson.M{
		"$set":   bson.M{"status": string(domainsubagent.AgentMessageDelivered), "delivered_at": deliveredAt},
		"$unset": bson.M{"claim_owner_id": "", "claim_expires_at": ""},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrTaskStateConflict
	}
	return nil
}

func (repository *Repository) MarkAgentMessageFailed(ctx context.Context, messageID string, reason string) error {
	result, err := repository.template.UpdateOne(ctx, messagesCollection, bson.M{"_id": strings.TrimSpace(messageID)}, bson.M{
		"$set": bson.M{"status": string(domainsubagent.AgentMessageFailed), "last_error": strings.TrimSpace(reason)},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrNotFound
	}
	return nil
}

// Acquire claims a task/run lease with one atomic MongoDB update. The task ID
// is the unique document key, so concurrent application instances cannot both
// become owners. A current owner may reacquire its lease during recovery.
func (repository *Repository) Acquire(ctx context.Context, taskID, runID, ownerID string, ttl time.Duration) error {
	taskID, runID, ownerID = strings.TrimSpace(taskID), strings.TrimSpace(runID), strings.TrimSpace(ownerID)
	if taskID == "" || runID == "" || ownerID == "" || ttl <= 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	now := time.Now().UTC()
	filter := bson.M{"_id": taskID, "$or": bson.A{
		bson.M{"expires_at": bson.M{"$lte": now}},
		bson.M{"expires_at": bson.M{"$exists": false}},
		bson.M{"owner_id": ownerID, "run_id": runID},
	}}
	update := bson.M{"$set": bson.M{
		"task_id": taskID, "run_id": runID, "owner_id": ownerID,
		"expires_at": now.Add(ttl), "updated_at": now,
	}}
	result, err := repository.template.UpdateOne(ctx, leasesCollection, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		// A duplicate-key error means another instance inserted the lease between
		// our filter and upsert. It is an expected admission conflict.
		if gomongo.IsDuplicateKeyError(err) {
			return subagentport.ErrExecutionLeaseNotAcquired
		}
		return err
	}
	if result.MatchedCount == 0 && result.UpsertedCount == 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	return nil
}

func (repository *Repository) Renew(ctx context.Context, taskID, runID, ownerID string, ttl time.Duration) error {
	taskID, runID, ownerID = strings.TrimSpace(taskID), strings.TrimSpace(runID), strings.TrimSpace(ownerID)
	if taskID == "" || runID == "" || ownerID == "" || ttl <= 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	now := time.Now().UTC()
	result, err := repository.template.UpdateOne(ctx, leasesCollection, bson.M{
		"_id": taskID, "run_id": runID, "owner_id": ownerID,
		"expires_at": bson.M{"$gt": now},
	}, bson.M{"$set": bson.M{"expires_at": now.Add(ttl), "updated_at": now}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	return nil
}

func (repository *Repository) Release(ctx context.Context, taskID, runID, ownerID string) error {
	taskID, runID, ownerID = strings.TrimSpace(taskID), strings.TrimSpace(runID), strings.TrimSpace(ownerID)
	if taskID == "" || runID == "" || ownerID == "" {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	now := time.Now().UTC()
	result, err := repository.template.UpdateOne(ctx, leasesCollection, bson.M{
		"_id": taskID, "run_id": runID, "owner_id": ownerID,
	}, bson.M{"$set": bson.M{"expires_at": now, "updated_at": now}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return subagentport.ErrExecutionLeaseNotAcquired
	}
	return nil
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
