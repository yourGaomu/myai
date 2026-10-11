package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	gomongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"myai/core/adapter/persistence/mongo/subagent/mapper"
	"myai/core/adapter/persistence/mongo/subagent/po"
	subagentport "myai/core/port/subagent"
)

var _ subagentport.TaskEventHistoryRepository = (*Repository)(nil)

func (repository *Repository) EnsureInspectionIndexes(ctx context.Context) error {
	if repository.database == nil {
		return errors.New("mongo subagent database is nil")
	}
	// 1. 回放必须按范围和序号检索，避免每次打开详情都扫描整个事件集合。
	_, err := repository.database.Collection(eventsCollection).Indexes().CreateMany(ctx, []gomongo.IndexModel{
		{Keys: bson.D{{Key: "sequence", Value: 1}}, Options: options.Index().SetName("subagent_events_sequence")},
		{Keys: bson.D{{Key: "task.parent_session_id", Value: 1}, {Key: "sequence", Value: 1}}, Options: options.Index().SetName("subagent_events_parent_sequence")},
		{Keys: bson.D{{Key: "task.parent_session_id", Value: 1}, {Key: "task._id", Value: 1}, {Key: "sequence", Value: 1}}, Options: options.Index().SetName("subagent_events_task_sequence")},
		{Keys: bson.D{{Key: "task.parent_session_id", Value: 1}, {Key: "task._id", Value: 1}, {Key: "run_id", Value: 1}, {Key: "sequence", Value: 1}}, Options: options.Index().SetName("subagent_events_run_sequence")},
	})
	return err
}

func (repository *Repository) QueryTaskEvents(ctx context.Context, filter subagentport.TaskEventFilter) ([]subagentport.TaskEvent, error) {
	// 1. 父会话范围始终保留，TaskID 和 RunID 只进一步缩小查询范围。
	criteria := bson.M{
		"task.parent_session_id": filter.ParentSessionID,
		"sequence":               bson.M{"$gt": filter.AfterSequence, "$lte": filter.ThroughSequence},
	}
	if filter.TaskID != "" {
		criteria["task._id"] = filter.TaskID
	}
	if filter.RunID != "" {
		criteria["run_id"] = filter.RunID
	}
	limit := filter.Limit
	if limit <= 0 || limit > 201 {
		limit = 201
	}
	var documents []po.TaskEventDocument
	if err := repository.template.FindAll(ctx, eventsCollection, criteria, &documents,
		options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}}).SetLimit(int64(limit))); err != nil {
		return nil, err
	}
	items := make([]subagentport.TaskEvent, 0, len(documents))
	for _, document := range documents {
		items = append(items, mapper.TaskEventDomainFromDocument(document))
	}
	return items, nil
}
