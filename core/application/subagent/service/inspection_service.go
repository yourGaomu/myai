package service

import (
	"context"
	"errors"
	"strings"

	subagentapi "myai/core/application/subagent/api"
	subagentquery "myai/core/application/subagent/query"
	subagentresult "myai/core/application/subagent/result"
	domainsubagent "myai/core/domain/subagent"
	repository "myai/core/port/repository"
	subagentport "myai/core/port/subagent"
	"myai/core/session"
)

var _ subagentapi.InspectionService = (*Service)(nil)

func (service *Service) inspectionTask(ctx context.Context, parentID, taskID string) (domainsubagent.Task, error) {
	if service == nil || service.Tasks == nil {
		return domainsubagent.Task{}, errors.New("subagent task repository is unavailable")
	}
	parentID, taskID = strings.TrimSpace(parentID), strings.TrimSpace(taskID)
	if parentID == "" || taskID == "" {
		return domainsubagent.Task{}, errors.New("parent session id and task id are required")
	}
	task, err := service.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return domainsubagent.Task{}, err
	}
	if task.ParentSessionID != parentID {
		return domainsubagent.Task{}, subagentport.ErrNotFound
	}
	return task, nil
}

func (service *Service) InspectTask(ctx context.Context, query subagentquery.InspectTask) (subagentresult.Inspection, error) {
	// 1. 强制提供父会话；由任务记录解析 ChildSessionID，不信任客户端传入的子会话。
	task, err := service.inspectionTask(ctx, query.ParentSessionID, query.TaskID)
	if err != nil {
		return subagentresult.Inspection{}, err
	}
	if service.SessionRecords == nil || service.Runs == nil || service.EventHistory == nil {
		return subagentresult.Inspection{}, errors.New("subagent inspection is not configured")
	}
	result := subagentresult.Inspection{Task: task}
	// 2. 先读取事件水位，再读取详情；查询期间新增的事件可从此水位继续补取。
	result.EventSequence, err = service.EventHistory.LatestTaskEventSequence(ctx)
	if err != nil {
		return subagentresult.Inspection{}, err
	}
	result.Runs, err = service.Runs.ListRuns(ctx, task.ID)
	if err != nil {
		return subagentresult.Inspection{}, err
	}
	// 3. 排队或启动失败时子会话可能尚未创建，此时仍可展示任务和事件。
	if strings.TrimSpace(task.ChildSessionID) == "" {
		return result, nil
	}
	record, err := service.SessionRecords.GetSession(ctx, task.ChildSessionID)
	if errors.Is(err, repository.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return subagentresult.Inspection{}, err
	}
	// 3.1 校验双向关系，避免错误关联的会话历史被当作子代理内容返回。
	if record.ID != task.ChildSessionID || record.Kind != string(session.KindSubagent) || record.ParentSessionID != task.ParentSessionID || record.ParentTaskID != task.ID {
		return subagentresult.Inspection{}, subagentport.ErrNotFound
	}
	result.SessionReady = true
	return result, nil
}

func (service *Service) ListTaskEvents(ctx context.Context, query subagentquery.TaskEvents) (subagentresult.EventPage, error) {
	// 1. 可以补取整个父会话，也可以仅查询某个子任务或其某次 Run。
	query.ParentSessionID = strings.TrimSpace(query.ParentSessionID)
	query.TaskID, query.RunID = strings.TrimSpace(query.TaskID), strings.TrimSpace(query.RunID)
	if query.ParentSessionID == "" {
		return subagentresult.EventPage{}, errors.New("parent session id is required")
	}
	if service == nil || service.EventHistory == nil {
		return subagentresult.EventPage{}, errors.New("subagent event history is unavailable")
	}
	if query.RunID != "" && query.TaskID == "" {
		return subagentresult.EventPage{}, errors.New("task id is required when filtering by run")
	}
	if query.TaskID != "" {
		if _, err := service.inspectionTask(ctx, query.ParentSessionID, query.TaskID); err != nil {
			return subagentresult.EventPage{}, err
		}
	}
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 200 {
		query.Limit = 200
	}
	// 2. 固定这一轮回放的上界，持续输出不会让分页永远追不到末尾。
	through, err := service.EventHistory.LatestTaskEventSequence(ctx)
	if err != nil {
		return subagentresult.EventPage{}, err
	}
	if query.ThroughSequence != 0 && query.ThroughSequence < through {
		through = query.ThroughSequence
	}
	if query.AfterSequence > through {
		return subagentresult.EventPage{}, errors.New("event cursor is ahead of history; restart replay from zero")
	}
	page := subagentresult.EventPage{Events: []subagentport.TaskEvent{}, NextSequence: through, ThroughSequence: through}
	if through == 0 || query.AfterSequence == through {
		return page, nil
	}
	events, err := service.EventHistory.QueryTaskEvents(ctx, subagentport.TaskEventFilter{
		ParentSessionID: query.ParentSessionID, TaskID: query.TaskID, RunID: query.RunID,
		AfterSequence: query.AfterSequence, ThroughSequence: through, Limit: query.Limit + 1,
	})
	if err != nil {
		return subagentresult.EventPage{}, err
	}
	// 3. 多读取一条判断是否还有下一页，游标只推进到已返回的数据。
	if len(events) > query.Limit {
		page.HasMore = true
		events = events[:query.Limit]
		page.NextSequence = events[len(events)-1].Sequence
	}
	page.Events = events
	return page, nil
}
