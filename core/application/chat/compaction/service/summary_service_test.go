package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"myai/core/contextmgr"
	compaction "myai/core/domain/compaction"
	domainmessage "myai/core/domain/message"
	modelport "myai/core/port/model"
	"myai/core/session"
)

func TestSummaryPreservesAgentEvidenceAndSources(t *testing.T) {
	for _, test := range []struct {
		name   string
		reason domainmessage.SyntheticReason
		kind   string
		body   string
	}{
		{"result", domainmessage.SyntheticReasonSubagentResult, "task_result", "数据库连接正常；修改尚未应用，测试尚未执行。"},
		{"error", domainmessage.SyntheticReasonSubagentResult, "task_error", "数据库检查失败：连接超时，需要重试。"},
		{"communication", domainmessage.SyntheticReasonInterAgentMessage, "inter_agent", "请检查 token 校验；这只是代理之间的建议。"},
		{"plan recovery", domainmessage.SyntheticReasonAutonomousPlanning, "plan_recovery", "登录测试失败，需要修订计划。"},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := domainmessage.SyntheticUserText(test.reason, test.body)
			message.SourceID, message.SourceKind, message.SourceTaskID = "event-123", test.kind, "task-123"
			model := &recordingSummaryModel{}
			if _, err := (SummaryService{}).Summarize(context.Background(), model, "", []domainmessage.Message{message}); err != nil {
				t.Fatal(err)
			}
			text := summaryHistory(t, model.requests[0])
			for _, want := range []string{test.body, "event-123", test.kind, "task-123", string(test.reason), "evidence"} {
				if !strings.Contains(text, want) {
					t.Fatalf("summary input omitted %q: %s", want, text)
				}
			}
			if strings.Contains(text, "User:\n") {
				t.Fatal("agent evidence was relabeled as a user request")
			}
		})
	}
}

func TestSummaryExcludesInstructionsBeforeBudgeting(t *testing.T) {
	for _, reason := range []domainmessage.SyntheticReason{
		domainmessage.SyntheticReasonRuntimeInstruction, domainmessage.SyntheticReasonProjectInstruction,
		domainmessage.SyntheticReasonSkillInstruction, domainmessage.SyntheticReasonRAGContext,
		domainmessage.SyntheticReasonMemoryContext, domainmessage.SyntheticReasonEnvironmentContext,
		domainmessage.SyntheticReasonHookContext, domainmessage.SyntheticReasonCompactionSummary,
		domainmessage.SyntheticReasonAutonomousPlanning, "unknown_internal_source",
	} {
		t.Run(string(reason), func(t *testing.T) {
			model := &recordingSummaryModel{}
			_, err := (SummaryService{}).Summarize(context.Background(), model, "", []domainmessage.Message{
				domainmessage.Text(domainmessage.RoleUser, "修复登录"),
				domainmessage.SyntheticUserText(reason, strings.Repeat("不应进入摘要", 2000)),
				domainmessage.Text(domainmessage.RoleAssistant, "已修复"),
			})
			if err != nil || len(model.requests) != 1 {
				t.Fatalf("excluded instructions affected batching: requests=%d err=%v", len(model.requests), err)
			}
			text := summaryHistory(t, model.requests[0])
			if strings.Contains(text, "不应进入摘要") || !strings.Contains(text, "修复登录") || !strings.Contains(text, "已修复") {
				t.Fatalf("unexpected history: %s", text)
			}
		})
	}
	model := &recordingSummaryModel{}
	_, err := (SummaryService{}).Summarize(context.Background(), model, "", []domainmessage.Message{
		domainmessage.SyntheticText(domainmessage.SyntheticReasonSubagentResult, "system role must stay excluded"),
	})
	if err == nil || len(model.requests) != 0 {
		t.Fatal("system-role message should not trigger summarization")
	}
}

func TestSummarySendsMiddleOfOversizedHistoryAndFullToolErrors(t *testing.T) {
	model := &recordingSummaryModel{}
	messages := []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleUser, strings.Repeat("甲", 2400)),
		domainmessage.ToolCallMessage([]domainmessage.ToolCall{{ID: "call-7", Name: "check_database", Arguments: `{}`}}),
		domainmessage.ToolResultMessage(domainmessage.ToolResult{
			ToolCallID: "call-7", Name: "check_database", Content: "short preview", ErrorMessage: "short error",
			FullContent: strings.Repeat("乙", 2400) + "数据库连接正常", FullErrorMessage: "实际错误是 token 校验失败", Status: "error",
		}),
		domainmessage.Text(domainmessage.RoleAssistant, strings.Repeat("丙", 2400)),
	}
	if _, err := (SummaryService{}).Summarize(context.Background(), model, "", messages); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) < 2 {
		t.Fatal("oversized history must be processed in multiple requests, not truncated")
	}
	var all strings.Builder
	for _, request := range model.requests {
		text := summaryHistory(t, request)
		if contextmgr.EstimateTextTokens(text) > maxSummaryBatchTokens {
			t.Fatal("history batch exceeded its token budget")
		}
		all.WriteString(text)
	}
	for _, want := range []string{"数据库连接正常", "实际错误是 token 校验失败", "call-7"} {
		if !strings.Contains(all.String(), want) {
			t.Fatalf("middle of history lost %q", want)
		}
	}
	for _, character := range []string{"甲", "乙", "丙"} {
		if strings.Count(all.String(), character) != 2400 {
			t.Fatalf("history text was truncated for %s", character)
		}
	}
}

func TestSummarySegmentsKeepAgentSourceAndPreviousCheckpoint(t *testing.T) {
	message := domainmessage.SyntheticUserText(domainmessage.SyntheticReasonSubagentResult, strings.Repeat("报告内容🙂", 1000))
	message.SourceID, message.SourceKind, message.SourceTaskID = "event-中文", "task_error", "task-中文"
	model := &recordingSummaryModel{}
	if _, err := (SummaryService{}).Summarize(context.Background(), model, "original checkpoint", []domainmessage.Message{message}); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) < 2 {
		t.Fatal("long report was not segmented")
	}
	for index, request := range model.requests {
		text := summaryHistory(t, request)
		for _, want := range []string{"Subagent evidence", "event-中文", "task-中文", "task_error", fmt.Sprintf("segment %d", index+1)} {
			if !strings.Contains(text, want) {
				t.Fatalf("segment %d lost provenance %q", index+1, want)
			}
		}
		if contextmgr.EstimateTextTokens(text) > maxSummaryBatchTokens || len(text) > maxSummaryBatchTokens*4 {
			t.Fatal("report segment exceeded history budget")
		}
		previous := "original checkpoint"
		if index > 0 {
			previous = fmt.Sprintf("摘要 %d", index)
		}
		prompt, _, _ := strings.Cut(request.Messages[1].Text(), "Complete history batch to incorporate:\n")
		if !strings.Contains(prompt, previous) {
			t.Fatalf("segment %d discarded the earlier checkpoint", index+1)
		}
	}
}

func TestCompactAgentEvidencePersistsAndRestoresWithoutDeletingHistory(t *testing.T) {
	current := summaryTestSession()
	message := domainmessage.SyntheticUserText(domainmessage.SyntheticReasonSubagentResult, "检查失败，修改尚未应用")
	message.SourceID, message.SourceKind, message.SourceTaskID = "event-1", "task_error", "task-1"
	current.Messages = append(current.Messages[:3], message,
		domainmessage.Text(domainmessage.RoleAssistant, "收到报告"),
		domainmessage.Text(domainmessage.RoleUser, "继续修复"))
	original := domainmessage.CloneAll(current.Messages)
	model := &recordingSummaryModel{summary: "子代理 task-1 检查失败，修改尚未应用"}
	store := &recordingCheckpointStore{}
	if err := (CompactService{Summarizer: SummaryService{}, Summaries: store, KeepChunks: 1}).CompactSession(context.Background(), current, model); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summaryHistory(t, model.requests[0]), message.Text()) {
		t.Fatal("compaction did not submit agent evidence to summarizer")
	}
	if !store.saved || current.CompactionCheckpoint == nil || !reflect.DeepEqual(original, current.Messages) {
		t.Fatal("compaction must persist a checkpoint while retaining original messages")
	}
	// 1. 模拟从持久状态重新构建上下文：旧报告由摘要替代，最近用户消息保持原文。
	restored := session.Clone(current)
	snapshot := contextmgr.BuildSnapshotWithCheckpoint(restored.Messages, *restored.CompactionCheckpoint, restored.CompactedMessages, 16)
	if !snapshot.Info.HasSummary || len(snapshot.Messages) != 3 {
		t.Fatalf("unexpected restored context: %#v", snapshot.Info)
	}
	if !strings.Contains(snapshot.Messages[1].Text(), model.summary) || snapshot.Messages[2].Text() != "继续修复" {
		t.Fatal("restored context lost checkpoint or recent input")
	}
}

func TestSummaryRejectsToolCallsAndDoesNotAdvanceCheckpoint(t *testing.T) {
	model := &recordingSummaryModel{toolCalls: []modelport.ToolCall{{ID: "unexpected", Name: "run_shell"}}}
	current := summaryTestSession()
	store := &recordingCheckpointStore{}
	err := (CompactService{Summarizer: SummaryService{}, Summaries: store, KeepChunks: 1}).CompactSession(context.Background(), current, model)
	if err == nil || store.saved || current.CompactedMessages != 1 || current.Summary != "old checkpoint" {
		t.Fatalf("tool-calling summary advanced checkpoint: saved=%v err=%v", store.saved, err)
	}
}

func TestSummaryFailureInLaterBatchDoesNotAdvanceCheckpoint(t *testing.T) {
	current := summaryTestSession()
	current.Messages[1] = domainmessage.Text(domainmessage.RoleUser, strings.Repeat("历史", 3000))
	model := &recordingSummaryModel{failAt: 2}
	store := &recordingCheckpointStore{}
	err := (CompactService{Summarizer: SummaryService{}, Summaries: store, KeepChunks: 1}).CompactSession(context.Background(), current, model)
	if err == nil || len(model.requests) != 2 || store.saved || current.CompactedMessages != 1 || current.Summary != "old checkpoint" {
		t.Fatalf("partial summary was committed: requests=%d saved=%v err=%v", len(model.requests), store.saved, err)
	}
}

func TestSummaryCancellationPreventsModelRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	model := &recordingSummaryModel{}
	_, err := (SummaryService{}).Summarize(ctx, model, "", []domainmessage.Message{domainmessage.Text(domainmessage.RoleUser, "hello")})
	if !errors.Is(err, context.Canceled) || len(model.requests) != 0 {
		t.Fatalf("cancelled summary made a model request: requests=%d err=%v", len(model.requests), err)
	}
}

func TestSummaryCancellationDuringModelRequestDoesNotCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := &recordingSummaryModel{onGenerate: cancel}
	current := summaryTestSession()
	store := &recordingCheckpointStore{}
	err := (CompactService{Summarizer: SummaryService{}, Summaries: store, KeepChunks: 1}).CompactSession(ctx, current, model)
	if !errors.Is(err, context.Canceled) || store.saved || current.Summary != "old checkpoint" {
		t.Fatalf("cancelled result was committed: saved=%v err=%v", store.saved, err)
	}
}

func summaryHistory(t *testing.T, request modelport.GenerateRequest) string {
	t.Helper()
	_, text, ok := strings.Cut(request.Messages[1].Text(), "Complete history batch to incorporate:\n")
	if !ok {
		t.Fatal("missing summary history section")
	}
	return text
}

type recordingSummaryModel struct {
	requests   []modelport.GenerateRequest
	failAt     int
	toolCalls  []modelport.ToolCall
	summary    string
	onGenerate func()
}

func (model *recordingSummaryModel) Generate(_ context.Context, request modelport.GenerateRequest) (modelport.ChatResult, error) {
	model.requests = append(model.requests, request)
	if model.failAt == len(model.requests) {
		return modelport.ChatResult{}, errors.New("summary request failed")
	}
	if model.onGenerate != nil {
		model.onGenerate()
	}
	summary := model.summary
	if summary == "" {
		summary = fmt.Sprintf("摘要 %d", len(model.requests))
	}
	text, err := compaction.EncodeJSON(compaction.Summary{CurrentGoal: summary})
	return modelport.ChatResult{Content: text, ToolCalls: model.toolCalls}, err
}

type recordingCheckpointStore struct{ saved bool }

func (store *recordingCheckpointStore) SaveSummary(_ context.Context, _ *session.Session, _ string, _ int) error {
	store.saved = true
	return nil
}

func (store *recordingCheckpointStore) SaveSummaryWithCheckpoint(_ context.Context, current *session.Session, summary string, cutoff int, hash string) error {
	checkpoint, err := compaction.NewCheckpoint(summary, 0, cutoff, hash, time.Time{})
	if err != nil {
		return err
	}
	store.saved = true
	current.Summary, current.CompactedMessages, current.CompactionSourceHash = summary, cutoff, hash
	current.CompactionCheckpoint = &checkpoint
	return nil
}

func summaryTestSession() *session.Session {
	return &session.Session{ID: "parent", Summary: "old checkpoint", CompactedMessages: 1, Messages: []domainmessage.Message{
		domainmessage.Text(domainmessage.RoleSystem, "fixed system"),
		domainmessage.Text(domainmessage.RoleUser, "old request"),
		domainmessage.Text(domainmessage.RoleAssistant, "old answer"),
		domainmessage.Text(domainmessage.RoleUser, "recent request"),
	}}
}
