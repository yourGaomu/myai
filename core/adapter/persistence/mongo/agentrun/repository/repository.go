package repository

import (
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/agentrun/mapper"
	"myai/core/adapter/persistence/mongo/agentrun/po"
	mongotemplate "myai/core/adapter/persistence/mongo/template"
	domainagentrun "myai/core/domain/agentrun"
	agentrunport "myai/core/port/agentrun"
)

const (
	runsCollection   = "agent_runs"
	eventsCollection = "agent_run_events"
)

type Repository struct {
	template mongotemplate.Operations
	database *gomongo.Database
}

var _ agentrunport.Repository = (*Repository)(nil)

func New(client *gomongo.Client, database string) *Repository {
	if client == nil || strings.TrimSpace(database) == "" {
		return &Repository{template: mongotemplate.New(nil)}
	}
	target := client.Database(database)
	return &Repository{template: mongotemplate.New(target), database: target}
}

func (r *Repository) SaveRun(ctx context.Context, run domainagentrun.Run) error {
	document := mapper.RunDocumentFromDomain(run)
	set := bson.M{
		"request_id": document.RequestID, "session_id": document.SessionID, "kind": document.Kind,
		"title": document.Title, "reason": document.Reason, "status": document.Status,
		"current_step": document.CurrentStep, "total_steps": document.TotalSteps,
		"last_sequence": document.LastSequence, "error_message": document.ErrorMessage,
		"started_at": document.StartedAt,
	}
	update := bson.M{"$set": set, "$setOnInsert": bson.M{"_id": document.ID}}
	if document.FinishedAt == nil {
		update["$unset"] = bson.M{"finished_at": ""}
	} else {
		set["finished_at"] = document.FinishedAt
	}
	_, err := r.template.UpdateOne(ctx, runsCollection, bson.M{"_id": document.ID}, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (r *Repository) GetRun(ctx context.Context, runID string) (domainagentrun.Run, error) {
	var document po.RunDocument
	if err := r.template.FindOne(ctx, runsCollection, bson.M{"_id": strings.TrimSpace(runID)}, &document); err != nil {
		return domainagentrun.Run{}, translateError(err)
	}
	return mapper.RunDomainFromDocument(document), nil
}

func (r *Repository) NextEventSequence(ctx context.Context, runID string) (int64, error) {
	if r == nil || r.database == nil {
		return 0, errors.New("mongo agent run database is nil")
	}
	var document po.RunDocument
	err := r.database.Collection(runsCollection).FindOneAndUpdate(
		ctx,
		bson.M{"_id": strings.TrimSpace(runID)},
		bson.M{"$inc": bson.M{"last_sequence": 1}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&document)
	if err != nil {
		return 0, translateError(err)
	}
	return document.LastSequence, nil
}

func (r *Repository) SaveEvent(ctx context.Context, event domainagentrun.Event) error {
	_, err := r.template.InsertOne(ctx, eventsCollection, mapper.EventDocumentFromDomain(event))
	return err
}

func (r *Repository) ReplaceEventContent(ctx context.Context, runID string, eventID string, content string, truncated bool) error {
	result, err := r.template.UpdateOne(ctx, eventsCollection, bson.M{
		"_id": strings.TrimSpace(eventID), "run_id": strings.TrimSpace(runID),
	}, bson.M{"$set": bson.M{"content": content, "truncated": truncated}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return agentrunport.ErrNotFound
	}
	return nil
}

func (r *Repository) ListRuns(ctx context.Context, sessionID string, limit int) ([]domainagentrun.Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var documents []po.RunDocument
	if err := r.template.FindAll(ctx, runsCollection, bson.M{"session_id": strings.TrimSpace(sessionID)}, &documents,
		options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]domainagentrun.Run, 0, len(documents))
	for index := len(documents) - 1; index >= 0; index-- {
		items = append(items, mapper.RunDomainFromDocument(documents[index]))
	}
	return items, nil
}

func (r *Repository) ListEvents(ctx context.Context, runIDs []string) ([]domainagentrun.Event, error) {
	if len(runIDs) == 0 {
		return []domainagentrun.Event{}, nil
	}
	var documents []po.EventDocument
	if err := r.template.FindAll(ctx, eventsCollection, bson.M{"run_id": bson.M{"$in": runIDs}}, &documents,
		options.Find().SetSort(bson.D{{Key: "run_id", Value: 1}, {Key: "sequence", Value: 1}})); err != nil {
		return nil, err
	}
	items := make([]domainagentrun.Event, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.EventDomainFromDocument(document))
	}
	return items, nil
}

func translateError(err error) error {
	if errors.Is(err, mongotemplate.ErrNotFound) || errors.Is(err, gomongo.ErrNoDocuments) {
		return agentrunport.ErrNotFound
	}
	return err
}
