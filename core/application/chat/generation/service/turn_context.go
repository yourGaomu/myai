package service

import (
	"context"
	"fmt"
	"strings"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	"myai/core/session"
)

func (s AgentLoopService) appendMessages(ctx context.Context, current *session.Session, messages ...domainmessage.Message) error {
	appended, err := s.appendSessionMessages(current, messages...)
	if err != nil {
		return err
	}
	if err := s.persistMessages(ctx, current, appended); err != nil {
		discardRequestMetadata(s.Sessions, current, appended)
		return err
	}
	return nil
}

// Preserve executed evidence, but never let a rejected durable write provide
// proof of presentation or a context report to later requests.
func discardRequestMetadata(store any, current *session.Session, messages []domainmessage.Message) {
	var ids []string
	for _, message := range messages {
		if message.RequestContext != nil || len(message.ImageReceipts) != 0 {
			ids = append(ids, message.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	if discarder, ok := store.(interface{ DiscardRequestMetadata(string, []string) }); ok {
		discarder.DiscardRequestMetadata(current.ID, ids)
	}
	for index := range current.Messages {
		message := &current.Messages[index]
		for _, id := range ids {
			if message.ID == id {
				message.RequestContext = nil
				message.ImageReceipts = nil
				break
			}
		}
	}
}

func (s AgentLoopService) appendSessionMessages(current *session.Session, messages ...domainmessage.Message) ([]domainmessage.Message, error) {
	if s.Sessions != nil {
		snapshot, appended, err := s.Sessions.AppendMessagesTo(current.ID, messages...)
		if err != nil {
			return nil, err
		}
		replaceRunSnapshot(current, snapshot)
		return appended, nil
	}
	start := len(current.Messages)
	current.AppendMessages(messages...)
	return domainmessage.CloneAll(current.Messages[start:]), nil
}

func (s AgentLoopService) refreshSession(current *session.Session) error {
	if s.Sessions == nil {
		return nil
	}
	snapshot, err := s.Sessions.Snapshot(current.ID)
	if err == nil {
		replaceRunSnapshot(current, snapshot)
	}
	return err
}

func replaceRunSnapshot(current, snapshot *session.Session) {
	// A planning request may use a read-only mode without changing the durable
	// session preference. All other state comes from the current aggregate.
	mode := current.AgentMode
	*current = *snapshot
	current.AgentMode = mode
}

func (s AgentLoopService) persistMessages(ctx context.Context, current *session.Session, messages []domainmessage.Message) error {
	if len(messages) == 0 {
		return nil
	}
	if s.OnMessagesAccepted != nil {
		if err := s.OnMessagesAccepted(ctx, current, domainmessage.CloneAll(messages)); err != nil {
			return fmt.Errorf("accept appended messages: %w", err)
		}
	} else if s.OnMessagesAppended != nil {
		s.OnMessagesAppended(current, domainmessage.CloneAll(messages))
	}
	return nil
}

func (s AgentLoopService) appendPlanContext(ctx context.Context, current *session.Session, messages []domainmessage.Message) (bool, error) {
	// 1. 在已选历史中查找最新计划快照；状态未变时不重复注入。
	text := planSnapshot(current.CurrentPlan)
	previous := ""
	for _, message := range messages {
		if message.SyntheticReason == domainmessage.SyntheticReasonPlanContext {
			previous = message.Text()
		}
	}
	if text == "" && previous == "" {
		return false, nil
	}
	if text == "" {
		text = "Current execution plan snapshot:\nNo active plan."
	}
	if previous == text {
		return false, nil
	}
	// 2. 计划推进只追加状态事件，不修改 System Prompt 或已发送的历史前缀。
	if err := s.appendMessages(ctx, current, domainmessage.SyntheticUserText(domainmessage.SyntheticReasonPlanContext, text)); err != nil {
		return false, err
	}
	return true, nil
}

func planSnapshot(current *agentplan.Plan) string {
	if current == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Current execution plan snapshot:\n")
	fmt.Fprintf(&builder, "plan_id: %s\nstatus: %s\ngoal: %s\nsteps:\n", current.ID, current.Status, current.Goal)
	for _, step := range current.Steps {
		fmt.Fprintf(&builder, "- order: %d; status: %s; title: %s", step.Order, step.Status, step.Title)
		if description := strings.TrimSpace(step.Description); description != "" {
			fmt.Fprintf(&builder, "; description: %s", description)
		}
		builder.WriteByte('\n')
	}
	return contextmgr.TruncateTextToTokens(builder.String(), 512)
}
