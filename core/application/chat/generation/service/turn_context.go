package service

import (
	"fmt"
	"strings"

	"myai/core/contextmgr"
	domainmessage "myai/core/domain/message"
	agentplan "myai/core/plan"
	"myai/core/session"
)

func (s AgentLoopService) appendMessages(current *session.Session, messages ...domainmessage.Message) error {
	appended, err := s.appendSessionMessages(current, messages...)
	if err != nil {
		return err
	}
	s.persistMessages(current, appended)
	return nil
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

func (s AgentLoopService) persistMessages(current *session.Session, messages []domainmessage.Message) {
	if len(messages) > 0 && s.OnMessagesAppended != nil {
		s.OnMessagesAppended(current, domainmessage.CloneAll(messages))
	}
}

func (s AgentLoopService) appendPlanContext(current *session.Session, messages []domainmessage.Message) bool {
	// 1. 在已选历史中查找最新计划快照；状态未变时不重复注入。
	text := planSnapshot(current.CurrentPlan)
	previous := ""
	for _, message := range messages {
		if message.SyntheticReason == domainmessage.SyntheticReasonPlanContext {
			previous = message.Text()
		}
	}
	if text == "" && previous == "" {
		return false
	}
	if text == "" {
		text = "Current execution plan snapshot:\nNo active plan."
	}
	if previous == text {
		return false
	}
	// 2. 计划推进只追加状态事件，不修改 System Prompt 或已发送的历史前缀。
	s.appendMessages(current, domainmessage.SyntheticUserText(domainmessage.SyntheticReasonPlanContext, text))
	return true
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
