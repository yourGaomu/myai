package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	modeladapter "myai/core/adapter/model/langchaingo"
	messagemapper "myai/core/adapter/persistence/chatmessage/mapper"
	mongomapper "myai/core/adapter/persistence/mongo/mapper"
	memoryadapter "myai/core/adapter/session/memory"
	toolexecutor "myai/core/adapter/tool/executor"
	contextservice "myai/core/application/chat/context/service"
	command "myai/core/application/chat/generation/command"
	loadservice "myai/core/application/session/load/service"
	"myai/core/domain/generation"
	"myai/core/port/files"
	modelport "myai/core/port/model"
	"myai/core/port/repository"
	"myai/core/session"
	"myai/core/tool"
	"myai/core/tool/local"
)

func TestToolImageWirePrefixSurvivesTranscriptReload(t *testing.T) {
	for _, adapter := range []modelport.ProtocolAdapter{modeladapter.OpenAIChatCompletionsAdapter{}, modeladapter.MistralChatAdapter{}} {
		t.Run(string(adapter.Protocol()), func(t *testing.T) {
			photo := visionFixture(t, "tool-photo", "image/png", 5, 3)
			fileService := &visionFileService{assets: map[string]files.Download{photo.Asset.AssetID: photo}}
			registry := tool.NewRegisterTools()
			registry.Register(local.NewReadAssetToolWithFiles(fileService, fileService.lookup))
			var requests []struct {
				Messages []json.RawMessage `json:"messages"`
				Tools    json.RawMessage   `json:"tools"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, struct {
					Messages []json.RawMessage `json:"messages"`
					Tools    json.RawMessage   `json:"tools"`
				}{})
				if err := json.NewDecoder(r.Body).Decode(&requests[len(requests)-1]); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if len(requests) == 1 {
					fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"picture","type":"function","function":{"name":"read_asset","arguments":"{\"asset_id\":\"tool-photo\"}"}},{"index":1,"id":"missing","type":"function","function":{"name":"read_asset","arguments":"{\"asset_id\":\"not-attached\"}"}}]},"finish_reason":"tool_calls"}]}`)
				} else {
					fmt.Fprint(w, `data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"Done"},"finish_reason":"stop"}]}`)
				}
				fmt.Fprint(w, "\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			model, err := adapter.CreateModel(modelport.CreationConfig{APIKey: "test-only", BaseURL: server.URL + "/v1", ModelName: "vision"})
			if err != nil {
				t.Fatal(err)
			}
			current := session.NewFromState(session.InitialState{ID: "session", Model: "vision", ContextWindowK: 16})
			current.AddUserTurn("", "read the attached asset with a tool")
			loop := AgentLoopService{
				Contexts: contextservice.SnapshotService{}, Tools: visionTools{registry.LLMTools()},
				ToolExecutor: toolexecutor.Executor{Registry: registry}, AssetFiles: fileService, LookupSessionAsset: fileService.lookup,
			}
			run := func(current *session.Session) modelport.ChatResult {
				t.Helper()
				result, err := loop.Run(t.Context(), command.Run{Model: model, Session: current, Settings: generation.SystemDefaults(), ModelWindowTokens: 15555})
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			result := run(current)
			if result.Message == nil || len(result.Message.ImageReceipts) != 1 || result.Message.RequestContext.WindowTokens != 15555 {
				t.Fatal("successful image sample did not retain the projected image and exact request window")
			}
			memory := memoryadapter.NewStore("vision")
			if err := memory.RestoreSession(current); err != nil {
				t.Fatal(err)
			}
			committed, err := (ResponseCommitService{Memory: memory}).Commit(command.Commit{Session: current, Result: result})
			if err != nil {
				t.Fatal(err)
			}
			var records []repository.MessageRecord
			for _, record := range (messagemapper.Mapper{}).MemoryMessages(committed.Session) {
				records = append(records, mongomapper.MessageRecordFromDocument(mongomapper.MessageDocumentFromRecord(record)))
			}
			reloaded := session.Clone(committed.Session)
			reloaded.Messages = loadservice.MessagesFromRecords(records)
			if got := (contextservice.QueryService{Contexts: contextservice.SnapshotService{}}).Info(t.Context(), reloaded).RequestContext; !reflect.DeepEqual(got, result.Message.RequestContext) {
				t.Fatalf("reopened context = %+v, want %+v", got, result.Message.RequestContext)
			}
			reloaded.AppendMessages(imageMessage("explain it again", photo))
			run(reloaded)
			if len(requests) != 3 {
				t.Fatalf("got %d requests, want 3", len(requests))
			}
			before, after := requests[1], requests[2]
			if len(after.Messages) < len(before.Messages) || !reflect.DeepEqual(after.Messages[:len(before.Messages)], before.Messages) || !reflect.DeepEqual(before.Tools, after.Tools) {
				t.Fatal("reloaded request did not preserve the raw message prefix and tool definitions")
			}
			last := string(before.Messages[len(before.Messages)-1])
			if !strings.Contains(last, "Image from read_asset") || !strings.Contains(last, base64.StdEncoding.EncodeToString(photo.Data)) || !strings.Contains(string(before.Messages[len(before.Messages)-2]), `"tool_call_id":"missing"`) {
				t.Fatal("image did not follow all parallel tool results with its original bytes")
			}
			legacy := session.Clone(committed.Session)
			for index := range legacy.Messages {
				legacy.Messages[index].ImageReceipts = nil
			}
			legacy.AddUserTurn("", "continue the legacy history")
			run(legacy)
			for _, body := range requests[3].Messages {
				if strings.Contains(string(body), "data:image/") {
					t.Fatal("a legacy tool result without a receipt gained an image")
				}
			}
			fileService.assets[photo.Asset.AssetID] = visionFixture(t, photo.Asset.AssetID, "image/png", 8, 4)
			run(reloaded)
			for _, body := range requests[4].Messages {
				if strings.Contains(string(body), "data:image/") {
					t.Fatal("historical image was admitted with changed bytes")
				}
			}
		})
	}
}
