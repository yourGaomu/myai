package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"myai/core/adapter/chat/pendinginput"
	modeladapter "myai/core/adapter/model/langchaingo"
	messagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	mongomapper "myai/core/adapter/persistence/mongo/mapper"
	toolrecorder "myai/core/adapter/persistence/toolrecords/repository"
	memoryadapter "myai/core/adapter/session/memory"
	contextservice "myai/core/application/chat/context/service"
	command "myai/core/application/chat/generation/command"
	result "myai/core/application/chat/generation/result"
	service "myai/core/application/chat/generation/service"
	loadservice "myai/core/application/session/load/service"
	"myai/core/contextmgr"
	"myai/core/domain/generation"
	message "myai/core/domain/message"
	"myai/core/plan"
	modelport "myai/core/port/model"
	"myai/core/port/repository"
	"myai/core/session"
)

func TestConversationWirePrefixSurvivesMailboxPlanProgressAndReload(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(bodies) == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"等待子代理。\",\"reasoning_content\":\" tool reasoning \",\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"wait_agent\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"完成。\",\"reasoning_content\":\" final reasoning \"},\"finish_reason\":\"stop\"}]}\n\n")
		}
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10000,\"completion_tokens\":20,\"total_tokens\":10020,\"prompt_tokens_details\":{\"cached_tokens\":8192}}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	model, err := (modeladapter.OpenAIChatCompletionsAdapter{}).CreateModel(modelport.CreationConfig{
		APIKey: "local-test", BaseURL: server.URL + "/v1", ModelName: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	current := session.NewFromState(session.InitialState{ID: "parent", Model: "test-model", ContextWindowK: 128})
	current.CurrentPlan = &plan.Plan{ID: "plan-1", Goal: "检查结果", Status: plan.StatusRunning,
		Steps: []plan.Step{{Order: 1, Title: "检查", Status: plan.StepStatusRunning}}}
	current.AddUserTurn("完成任务后总结", "启动子代理并等待")
	store := &transcriptStore{}
	mapper := messagemapper.Mapper{}
	store.messages = mapper.MemoryMessages(current)
	queue := pendinginput.NewQueue()
	loop := service.AgentLoopService{
		Contexts: contextservice.SnapshotService{}, Tools: replayTools{}, PendingInput: queue,
		ToolExecutor: replayExecutor{queue: queue}, ToolRecords: toolrecorder.Recorder{Persistence: store},
		OnMessagesAppended: func(current *session.Session, messages []message.Message) {
			for _, record := range mapper.UserTurn(command.PersistUserMessage{SessionID: current.ID, AppendedMessages: messages}, time.Now()) {
				if err := store.SaveMessage(context.Background(), record); err != nil {
					t.Error(err)
				}
			}
		},
	}
	response, err := loop.Run(context.Background(), command.Run{Model: model, Session: current,
		Settings: generation.SystemDefaults(), EnvironmentContext: "Current time: 12:00"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests=%d", len(bodies))
	}
	first, second := wireMessages(t, bodies[0]), wireMessages(t, bodies[1])
	assertWirePrefix(t, first, second)
	assistant := second[len(first)].(map[string]any)
	if assistant["content"] != "等待子代理。" || assistant["reasoning_content"] != " tool reasoning " || assistant["tool_calls"] == nil {
		t.Fatalf("incomplete assistant replay: %#v", assistant)
	}
	if !reflect.DeepEqual(bodies[0]["tools"], bodies[1]["tools"]) {
		t.Fatal("tools changed during turn")
	}
	if response.Usage.PromptCachedTokens != 16384 {
		t.Fatalf("cached usage lost: %+v", response.Usage)
	}
	if response.Message == nil || response.Message.Reasoning != " final reasoning " {
		t.Fatal("final message contains aggregated reasoning")
	}
	if queue.HasPending(current.ID) {
		t.Fatal("mailbox not acknowledged")
	}
	memory := memoryadapter.NewStore("test-model")
	if err := memory.RestoreSession(current); err != nil {
		t.Fatal(err)
	}
	current, err = memory.GetSession(current.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (service.ResponseCommitService{Memory: memory}).Commit(command.Commit{Session: current, Result: response})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessage(context.Background(), mapper.AssistantMessageFromSession(current, response, time.Now())); err != nil {
		t.Fatal(err)
	}

	// 1. 数据库扁平记录经过 Mongo 映射再恢复，必须与实际发送的消息结构一致。
	restored := session.Clone(current)
	restored.Messages = loadservice.MessagesFromRecords(store.messages)
	if contextmgr.StableMessagesHash(current.Messages) != contextmgr.StableMessagesHash(restored.Messages) {
		t.Fatal("reloaded transcript changed model-visible content")
	}
	if contextmgr.CompactionSourceHash(current.Messages, len(current.Messages)) != contextmgr.CompactionSourceHash(restored.Messages, len(restored.Messages)) {
		t.Fatal("reload lost source metadata or changed checkpoint hash")
	}
	if !reflect.DeepEqual(mapper.MemoryMessages(current), mapper.MemoryMessages(restored)) {
		t.Fatal("reload changed durable message IDs or record ordering")
	}
	// 2. 下一条用户消息携带新的时间；上一轮环境和计划快照仍保留在原位置。
	restored.AddUserTurn("完成任务后总结", "进一步解释")
	_, err = loop.Run(context.Background(), command.Run{Model: model, Session: restored,
		Settings: generation.SystemDefaults(), EnvironmentContext: "Current time: 12:01"})
	if err != nil {
		t.Fatal(err)
	}
	third := wireMessages(t, bodies[2])
	assertWirePrefix(t, second, third)
	final := third[len(second)].(map[string]any)
	if final["reasoning_content"] != " final reasoning " {
		t.Fatalf("final reasoning replay=%v", final)
	}
}

func wireMessages(t *testing.T, body map[string]any) []any {
	t.Helper()
	messages, ok := body["messages"].([]any)
	if !ok {
		t.Fatalf("missing messages: %v", body)
	}
	return messages
}

func assertWirePrefix(t *testing.T, before, after []any) {
	t.Helper()
	if len(after) < len(before) || !reflect.DeepEqual(before, after[:len(before)]) {
		t.Fatalf("wire prefix changed\nbefore=%v\nafter=%v", before, after)
	}
}

type transcriptStore struct{ messages []repository.MessageRecord }

func (s *transcriptStore) SaveMessage(_ context.Context, record repository.MessageRecord) error {
	for _, existing := range s.messages {
		if existing.ID == record.ID {
			return fmt.Errorf("duplicate message ID %s", record.ID)
		}
	}
	s.messages = append(s.messages, mongomapper.MessageRecordFromDocument(mongomapper.MessageDocumentFromRecord(record)))
	return nil
}
func (*transcriptStore) SaveAsset(context.Context, repository.AssetRecord) error { return nil }

type replayTools struct{ description string }

func (tools replayTools) ToolsForSession(*session.Session, bool) []modelport.Tool {
	return []modelport.Tool{{Type: "function", Function: &modelport.FunctionDefinition{
		Name: "wait_agent", Description: tools.description, Parameters: map[string]any{"type": "object"},
	}}}
}

type replayExecutor struct{ queue *pendinginput.Queue }

func (executor replayExecutor) Execute(_ context.Context, c command.ToolExecution) (result.ToolExecution, error) {
	if err := executor.queue.EnqueueIdentified(c.Session.ID, "child-result", "子代理检查通过"); err != nil {
		return result.ToolExecution{}, err
	}
	c.Session.CurrentPlan.Steps[0].Status = plan.StepStatusDone
	return result.ToolExecution{Calls: c.Calls, Messages: []message.Message{message.ToolResultMessage(message.ToolResult{
		ToolCallID: c.Calls[0].ID, Name: c.Calls[0].Name, Content: "done",
	})}}, nil
}

type countingModel struct{ calls int }

func (m *countingModel) Generate(context.Context, modelport.GenerateRequest) (modelport.ChatResult, error) {
	m.calls++
	return modelport.ChatResult{Content: "done"}, nil
}

func TestRequestBudgetIncludesToolsEnvironmentAndOutput(t *testing.T) {
	for _, kind := range []string{"tools", "environment", "output"} {
		t.Run(kind, func(t *testing.T) {
			current := &session.Session{ID: "budget", ContextWindowK: 4, Messages: []message.Message{
				message.Text(message.RoleSystem, "system"), message.Text(message.RoleUser, "hello"),
			}}
			model := &countingModel{}
			loop := service.AgentLoopService{Contexts: contextservice.SnapshotService{}}
			cmd := command.Run{Session: current, Model: model, Settings: generation.SystemDefaults()}
			switch kind {
			case "tools":
				loop.Tools = replayTools{description: strings.Repeat("工具说明", 2000)}
			case "environment":
				cmd.EnvironmentContext = strings.Repeat("环境信息", 2000)
			case "output":
				cmd.Settings.MaxOutputTokens = 4000
			}
			_, err := loop.Run(context.Background(), cmd)
			if err == nil || model.calls != 0 || !strings.Contains(err.Error(), "context window") {
				t.Fatalf("oversized request reached model: err=%v calls=%d", err, model.calls)
			}
		})
	}
}
