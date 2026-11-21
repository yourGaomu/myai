package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"reflect"
	"strings"
	"testing"

	memoryadapter "myai/core/adapter/session/memory"
	toolexecutor "myai/core/adapter/tool/executor"
	compactionresult "myai/core/application/chat/compaction/result"
	contextservice "myai/core/application/chat/context/service"
	command "myai/core/application/chat/generation/command"
	result "myai/core/application/chat/generation/result"
	toolcommand "myai/core/application/tool/command"
	toolresult "myai/core/application/tool/result"
	"myai/core/contextmgr"
	message "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/port/files"
	modelport "myai/core/port/model"
	"myai/core/session"
	"myai/core/tool"
	"myai/core/tool/local"
)

type overflowCompactor struct {
	calls   int
	windows []int
}

func (c *overflowCompactor) CompactIfNeeded(context.Context, *session.Session, modelport.ChatModelPort) (compactionresult.CompactInfo, error) {
	return compactionresult.CompactInfo{}, nil
}
func (c *overflowCompactor) CompactIfNeededForRequest(_ context.Context, current *session.Session, _ modelport.ChatModelPort, options contextmgr.RequestOptions) (compactionresult.CompactInfo, error) {
	c.calls++
	c.windows = append(c.windows, options.Budget.WindowTokens)
	if options.Budget.WindowTokens < 16000 {
		// This fake models a persisted checkpoint being projected by a provider.
		current.Messages = []message.Message{message.Text(message.RoleSystem, "system"), message.Text(message.RoleUser, "finish")}
	}
	return compactionresult.CompactInfo{}, nil
}

type recoveryTools struct{}

func (recoveryTools) ToolsForSession(*session.Session, bool) []modelport.Tool {
	return []modelport.Tool{{Type: "function", Function: &modelport.FunctionDefinition{Name: "read_file", Parameters: map[string]any{"type": "object"}}}}
}

type recoveryExecutor struct{ calls int }

func (e *recoveryExecutor) Execute(_ context.Context, c command.ToolExecution) (result.ToolExecution, error) {
	e.calls++
	return result.ToolExecution{Calls: c.Calls, Messages: []message.Message{message.ToolResultMessage(message.ToolResult{ToolCallID: c.Calls[0].ID, Name: "read_file", Content: "done"})}}, nil
}

func TestContextOverflowCompactsOnceWithoutReplayingExecutedTool(t *testing.T) {
	for _, repeated := range []bool{false, true} {
		t.Run(fmt.Sprint(repeated), func(t *testing.T) {
			model := &pendingModel{responses: []modelport.ChatResult{
				{ToolCalls: []message.ToolCall{{ID: "read-1", Name: "read_file", Arguments: `{}`}}}, {}, {Content: "finished"},
			}, errors: []error{nil, modelport.ErrContextWindowExceeded, nil}}
			if repeated {
				model.errors[2] = modelport.ErrContextWindowExceeded
			}
			compactor := &overflowCompactor{}
			executor := &recoveryExecutor{}
			current := &session.Session{ID: "session", Messages: []message.Message{message.Text(message.RoleSystem, "system"), message.Text(message.RoleUser, strings.Repeat("history ", 1000))}}
			response, err := (AgentLoopService{Contexts: contextservice.SnapshotService{}, Tools: recoveryTools{}, ToolExecutor: executor, Compactor: compactor}).Run(context.Background(), command.Run{Model: model, Session: current, ModelWindowTokens: 16000})
			if repeated {
				if !errors.Is(err, modelport.ErrContextWindowExceeded) {
					t.Fatalf("expected second overflow, got %v", err)
				}
			} else if err != nil || response.Content != "finished" {
				t.Fatalf("recovery response=%+v err=%v", response, err)
			}
			if model.calls != 3 || executor.calls != 1 {
				t.Fatalf("model calls=%d tool calls=%d", model.calls, executor.calls)
			}
			if len(compactor.windows) != 3 || compactor.windows[2] >= compactor.windows[1] {
				t.Fatalf("missing reduced-budget retry: %v", compactor.windows)
			}
		})
	}
}

func TestInvalidHistoricalToolJSONIsQuarantinedWithResultsWithoutMutatingStore(t *testing.T) {
	memory := memoryadapter.NewStore("model")
	history := []message.Message{
		message.Text(message.RoleSystem, "system"), message.Text(message.RoleUser, "old"),
		message.Assistant("old exchange", "", []message.ToolCall{{ID: "bad", Name: "read_file", Arguments: `{"path":`}, {ID: "sibling", Name: "read_file", Arguments: `{}`}}),
		message.ToolResultMessage(message.ToolResult{ToolCallID: "bad", Content: "bad old result"}),
		message.ToolResultMessage(message.ToolResult{ToolCallID: "sibling", Content: "sibling result"}),
		message.Text(message.RoleUser, "continue"),
	}
	if err := memory.PutSession("session", "model", history); err != nil {
		t.Fatal(err)
	}
	before, _ := memory.Snapshot("session")
	model := &pendingModel{responses: []modelport.ChatResult{{Content: "recovered"}}}
	_, err := (AgentLoopService{Sessions: memory, Contexts: contextservice.SnapshotService{}}).Run(context.Background(), command.Run{Model: model, Session: before})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range model.requests[0].Messages {
		if msg.HasToolCall() || msg.Role == message.RoleTool {
			t.Fatal("quarantined exchange left orphan tool messages")
		}
	}
	if !requestContainsText(model.requests[0], "invalid JSON") {
		t.Fatal("missing recovery context")
	}
	after, _ := memory.Snapshot("session")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("request projection mutated raw session")
	}
}

type failingStopHooks struct{ calls int }

func (h *failingStopHooks) Handle(context.Context, command.TurnHook) (command.TurnHookOutcome, error) {
	h.calls++
	return command.TurnHookOutcome{}, errors.New("hook failed")
}
func TestStopHookFailurePreservesAnswerAndInternalRunSkipsStop(t *testing.T) {
	for _, internal := range []bool{false, true} {
		t.Run(fmt.Sprint(internal), func(t *testing.T) {
			hooks := &failingStopHooks{}
			model := &pendingModel{responses: []modelport.ChatResult{{Content: "completed answer"}}}
			reported := 0
			response, err := (AgentLoopService{Contexts: contextservice.SnapshotService{}, TurnHooks: hooks, OnHookError: func(error) { reported++ }}).Run(context.Background(), command.Run{Session: &session.Session{ID: "session"}, Model: model, Internal: internal})
			if err != nil || response.Content != "completed answer" {
				t.Fatalf("answer lost: %+v %v", response, err)
			}
			expected := 1
			if internal {
				expected = 0
			}
			if hooks.calls != expected || reported != expected {
				t.Fatalf("hook=%d reported=%d", hooks.calls, reported)
			}
		})
	}
}

type visionModel struct{ pendingModel }

func (*visionModel) SupportsImageInput() bool { return true }

type visionFileService struct {
	files.Service
	assets      map[string]files.Download
	requests    []files.DownloadRequest
	downloadErr error
}

func (f *visionFileService) Download(_ context.Context, request files.DownloadRequest) (files.Download, error) {
	f.requests = append(f.requests, request)
	return f.assets[request.AssetID], f.downloadErr
}

func (f *visionFileService) lookup(_ context.Context, sessionID, id string) (files.Asset, error) {
	if sessionID == "session" {
		if asset, ok := f.assets[id]; ok {
			return asset.Asset, nil
		}
	}
	return files.Asset{}, files.ErrSessionAssetUnavailable
}

func visionFixture(t *testing.T, id, mimeType string, width, height int) files.Download {
	t.Helper()
	var data bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	var err error
	if mimeType == "image/jpeg" {
		err = jpeg.Encode(&data, picture, nil)
	} else {
		err = png.Encode(&data, picture)
	}
	if err != nil {
		t.Fatal(err)
	}
	return files.Download{Asset: files.Asset{AssetID: id, BusinessID: "files", ContentType: mimeType, Size: int64(data.Len())}, Data: data.Bytes()}
}

func uploadedAssetBlock(id, name, contentType string, size int64) string {
	return fmt.Sprintf("<uploaded_file asset_id=\"%s\" name=\"%s\" content_type=\"%s\" size=\"%d\">\n[User attachment]\n</uploaded_file>", id, name, contentType, size)
}

func imageMessage(prompt string, assets ...files.Download) message.Message {
	var blocks []string
	for _, asset := range assets {
		blocks = append(blocks, uploadedAssetBlock(asset.Asset.AssetID, "attach", asset.Asset.ContentType, asset.Asset.Size))
	}
	return message.Text(message.RoleUser, prompt+attachmentTailMarker+strings.Join(blocks, "\n\n"))
}

func TestSelectedAttachmentTailIgnoresEmbeddedNonUserAndUnselectedReferences(t *testing.T) {
	jpegAsset := visionFixture(t, "photo", "image/jpeg", 7, 5)
	block := uploadedAssetBlock("photo", "photo.jpg", "image/jpeg", jpegAsset.Asset.Size)
	fileBody := "workspace file contents" + attachmentTailMarker + block
	workspace := fmt.Sprintf("<file path=\"readme.md\" language=\"markdown\" size=\"%d\">\n%s\n\n</file>", len(fileBody), fileBody)
	original := imageMessage("Use the attachment", jpegAsset)
	synthetic := message.SyntheticUserText(message.SyntheticReasonHookContext, original.Text())
	oldSystem := message.Text(message.RoleSystem, original.Text())
	plain := message.Text(message.RoleUser, "no attached file")
	for _, tc := range []struct {
		name       string
		selected   []message.Message
		transcript []message.Message
		wantImage  bool
	}{
		{name: "valid", selected: []message.Message{original}, wantImage: true},
		{name: "only snapshot messages", selected: []message.Message{plain}, transcript: []message.Message{original, plain}},
		{name: "synthetic", selected: []message.Message{synthetic}},
		{name: "projected system", selected: []message.Message{original}, transcript: []message.Message{oldSystem}},
		{name: "quoted tag without tail", selected: []message.Message{message.Text(message.RoleUser, "Example:\n"+block)}},
		{name: "fenced block", selected: []message.Message{message.Text(message.RoleUser, "```"+attachmentTailMarker+block+"\n```")}},
		{name: "trailing content", selected: []message.Message{message.Text(message.RoleUser, original.Text()+"\n\ncontinue reading")}},
		{name: "workspace attachment body", selected: []message.Message{message.Text(message.RoleUser, "Files"+attachmentTailMarker+workspace)}},
		{name: "workspace and uploaded attachment", selected: []message.Message{message.Text(message.RoleUser, "Files"+attachmentTailMarker+workspace+"\n\n"+block)}, wantImage: true},
		{name: "unfinished past turn", selected: []message.Message{original, plain}},
		{name: "answered past turn", selected: []message.Message{original, message.Assistant("I saw it", "", nil), plain}, wantImage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transcript := tc.transcript
			if transcript == nil {
				transcript = tc.selected
			}
			fileService := &visionFileService{assets: map[string]files.Download{"photo": jpegAsset}}
			service := AgentLoopService{AssetFiles: fileService, LookupSessionAsset: fileService.lookup}
			request := modelport.GenerateRequest{Messages: tc.selected}
			snapshot := contextmgr.Snapshot{Messages: tc.selected}
			budget := contextmgr.NewRequestBudget(16000, 0, 1024)
			if _, err := service.addRequestImages(t.Context(), "session", &visionModel{}, &turnImages{}, snapshot, transcript, budget, &request); err != nil {
				t.Fatal(err)
			}
			if (len(request.Images) == 1) != tc.wantImage || (len(fileService.requests) == 1) != tc.wantImage {
				t.Fatalf("images=%d downloads=%d", len(request.Images), len(fileService.requests))
			}
			if tc.wantImage && (!bytes.Equal(request.Images[0].Bytes(), jpegAsset.Data) || request.Images[0].MIMEType != "image/jpeg") {
				t.Fatal("verified JPEG changed")
			}
		})
	}
}

type visionOverflowCompactor struct{}

func (visionOverflowCompactor) CompactIfNeeded(context.Context, *session.Session, modelport.ChatModelPort) (compactionresult.CompactInfo, error) {
	return compactionresult.CompactInfo{}, nil
}

func (visionOverflowCompactor) CompactIfNeededForRequest(_ context.Context, current *session.Session, _ modelport.ChatModelPort, options contextmgr.RequestOptions) (compactionresult.CompactInfo, error) {
	if options.Budget.WindowTokens < 16000 {
		current.Messages = current.Messages[len(current.Messages)-1:]
	}
	return compactionresult.CompactInfo{}, nil
}

func TestImageInputOverflowRebuildsBindingsWithoutDownloadingAgain(t *testing.T) {
	photo := visionFixture(t, "photo", "image/jpeg", 7, 5)
	current := &session.Session{ID: "session", Messages: []message.Message{
		message.Text(message.RoleUser, strings.Repeat("history ", 1200)),
		message.Assistant("previous answer", "", nil), imageMessage("What is this?", photo),
	}}
	fileService := &visionFileService{assets: map[string]files.Download{"photo": photo}}
	model := &visionModel{pendingModel: pendingModel{errors: []error{modelport.ErrContextWindowExceeded}}}
	service := AgentLoopService{Contexts: contextservice.SnapshotService{}, Compactor: visionOverflowCompactor{}, AssetFiles: fileService, LookupSessionAsset: fileService.lookup}
	if _, err := service.Run(t.Context(), command.Run{Session: current, Model: model, ModelWindowTokens: 16000}); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 2 || len(fileService.requests) != 1 {
		t.Fatalf("calls=%d downloads=%d", len(model.requests), len(fileService.requests))
	}
	for index, request := range model.requests {
		if len(request.Images) != 1 {
			t.Fatalf("sample %d images=%d", index, len(request.Images))
		}
		item := request.Images[0]
		if item.MessageIndex != 2-index*2 || !strings.HasPrefix(request.Messages[item.MessageIndex].Text(), "What is this?") || !bytes.Equal(item.Bytes(), photo.Data) {
			t.Fatalf("sample %d image binding = %v", index, item)
		}
	}
}

func TestImageInputReportsUnavailabilityAndEnforcesBudgets(t *testing.T) {
	photo := visionFixture(t, "photo", "image/jpeg", 7, 5)
	wrong := photo
	wrong.Data = []byte("echoed-private-data")
	wrong.Asset.Size = int64(len(wrong.Data))
	large := photo
	large.Asset.Size = modelport.MaxInputImageBytes + 1
	wide := visionFixture(t, "photo", "image/jpeg", 16385, 1)
	badChecksum := photo
	badChecksum.Asset.Checksum = strings.Repeat("0", 64)
	for _, tc := range []struct {
		name      string
		sessionID string
		model     modelport.ChatModelPort
		asset     files.Download
		window    int
		download  error
		wantReads int
		wantText  string
	}{
		{name: "cross-session", sessionID: "other", wantText: "unavailable in this session"},
		{name: "unsupported adapter", model: &pendingModel{}, wantText: "Chat Completions"},
		{name: "oversized", asset: large, wantText: "5 MiB limit"},
		{name: "resolution", asset: wide, wantReads: 1, wantText: "resolution limit"},
		{name: "context", window: 200, wantText: "context has insufficient space"},
		{name: "invalid bytes", asset: wrong, wantReads: 1, wantText: "not a complete supported image"},
		{name: "digest mismatch", asset: badChecksum, wantReads: 1, wantText: "did not match the attachment record"},
		{name: "unavailable file", download: errors.New("echoed-private-data"), wantReads: 1, wantText: "could not read the attached image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sessionID == "" {
				tc.sessionID = "session"
			}
			if tc.model == nil {
				tc.model = &visionModel{}
			}
			if tc.asset.Asset.AssetID == "" {
				tc.asset = photo
			}
			if tc.window == 0 {
				tc.window = 16000
			}
			reference := imageMessage("Look at this", tc.asset)
			messages := []message.Message{reference}
			req := modelport.GenerateRequest{Messages: messages}
			fileService := &visionFileService{assets: map[string]files.Download{"photo": tc.asset}, downloadErr: tc.download}
			service := AgentLoopService{AssetFiles: fileService, LookupSessionAsset: fileService.lookup}
			tokens, err := service.addRequestImages(t.Context(), tc.sessionID, tc.model, &turnImages{}, contextmgr.Snapshot{Messages: messages}, messages, contextmgr.NewRequestBudget(tc.window, 0, 0), &req)
			if err != nil || len(req.Images) != 0 || len(fileService.requests) != tc.wantReads || tokens != 0 {
				t.Fatalf("images=%d downloads=%d tokens=%d error=%v", len(req.Images), len(fileService.requests), tokens, err)
			}
			feedback := req.Messages[len(req.Messages)-1].Text()
			if !strings.Contains(feedback, "Image content was not provided") || !strings.Contains(feedback, tc.wantText) || strings.Contains(feedback, "echoed-private-data") {
				t.Fatalf("wrong or unsafe feedback: %q", feedback)
			}
		})
	}
}

func TestImageInputLimitsCountAndTotalDownloadBytes(t *testing.T) {
	for _, each := range []int{1024, 5 << 20} {
		t.Run(fmt.Sprint(each), func(t *testing.T) {
			fileService := &visionFileService{assets: make(map[string]files.Download)}
			var references []files.Download
			for index := range 5 {
				id := fmt.Sprintf("photo-%d", index)
				item := visionFixture(t, id, "image/jpeg", 7, 5)
				item.Data = append(item.Data, make([]byte, each-len(item.Data))...)
				item.Asset.Size = int64(each)
				fileService.assets[id] = item
				references = append(references, item)
			}
			service := AgentLoopService{AssetFiles: fileService, LookupSessionAsset: fileService.lookup}
			current := &session.Session{ID: "session", Messages: []message.Message{imageMessage("examine these", references...)}}
			model := &visionModel{}
			service.Contexts = contextservice.SnapshotService{}
			if _, err := service.Run(t.Context(), command.Run{Session: current, Model: model}); err != nil {
				t.Fatal(err)
			}
			request := model.requests[0]
			want := min(modelport.MaxInputImages, modelport.MaxTotalInputImageBytes/each)
			if len(request.Images) != want || len(fileService.requests) != want || !requestContainsText(request, "Image content was not provided") {
				t.Fatalf("images=%d downloads=%d want=%d", len(request.Images), len(fileService.requests), want)
			}
		})
	}
}

type visionRecordSink struct{ recorded []command.ToolExecutionRecord }

func (sink *visionRecordSink) RecordToolExecution(_ context.Context, record command.ToolExecutionRecord) {
	sink.recorded = append(sink.recorded, record)
}

type visionToolHook struct{ result string }

func (*visionToolHook) BeforeToolUse(_ context.Context, event toolcommand.HookEvent) (toolresult.Hook, error) {
	if event.Name == "read_asset" && strings.Contains(event.Arguments, `"rewrite-me"`) {
		return toolresult.Hook{Arguments: `{"asset_id":"extra"}`}, nil
	}
	return toolresult.Hook{}, nil
}

func (hook *visionToolHook) AfterToolUse(_ context.Context, event toolcommand.HookEvent) {
	if event.Name == "read_asset" && strings.Contains(event.Arguments, `"extra"`) {
		hook.result = event.Result
	}
}

func TestAgentLoopVisionReadAssetUsesExecutedCallsAndPreservesBatchAndRecords(t *testing.T) {
	photo, extra := visionFixture(t, "photo", "image/jpeg", 7, 5), visionFixture(t, "extra", "image/png", 5, 3)
	fileService := &visionFileService{assets: map[string]files.Download{"photo": photo, "extra": extra}}
	registry := tool.NewRegisterTools()
	registry.Register(local.NewReadAssetToolWithFiles(fileService, fileService.lookup))
	hook := &visionToolHook{}
	records := &visionRecordSink{}
	model := &visionModel{pendingModel: pendingModel{responses: []modelport.ChatResult{
		{ToolCalls: []message.ToolCall{
			{ID: "one", Name: "read_asset", Arguments: `{"asset_id":"rewrite-me"}`},
			{ID: "same", Name: "read_asset", Arguments: `{"asset_id":"photo"}`},
			{ID: "denied", Name: "read_asset", Arguments: `{"asset_id":"wrong-session"}`},
		}},
		{Content: "finished"},
	}}}
	memory := memoryadapter.NewStore("vision")
	if err := memory.PutSession("session", "vision", []message.Message{imageMessage("what do you see?", photo)}); err != nil {
		t.Fatal(err)
	}
	current, err := memory.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	service := AgentLoopService{
		Sessions: memory, Contexts: contextservice.SnapshotService{}, AssetFiles: fileService, LookupSessionAsset: fileService.lookup,
		ToolExecutor: toolexecutor.Executor{Registry: registry, Hooks: hook}, ToolRecords: records,
	}
	response, err := service.Run(t.Context(), command.Run{Session: current, Model: model})
	if err != nil || response.Content != "finished" || model.calls != 2 {
		t.Fatalf("response=%+v calls=%d err=%v", response, model.calls, err)
	}
	if len(fileService.requests) != 2 || fileService.requests[0].AssetID != "photo" || fileService.requests[1].AssetID != "extra" {
		t.Fatalf("duplicate or unauthorized downloads: %+v", fileService.requests)
	}
	first, next := model.requests[0], model.requests[1]
	if len(first.Images) != 1 || len(next.Images) != 2 {
		t.Fatalf("images first=%d next=%d", len(first.Images), len(next.Images))
	}
	if err := contextmgr.ValidateToolPairs(next.Messages); err != nil {
		t.Fatal(err)
	}
	if target := next.Messages[next.Images[0].MessageIndex]; target.Role != message.RoleUser || target.IsSynthetic() || !strings.HasPrefix(target.Text(), "what do you see?") {
		t.Fatal("user attachment moved from its original message")
	}
	index := next.Images[1].MessageIndex
	lastTool, ok := next.Messages[index-1].FirstToolResult()
	if !ok || lastTool.ToolCallID != "denied" || lastTool.Status == domaintool.ResultStatusSuccess || !strings.Contains(next.Messages[index].Text(), "asset_id: extra") || !bytes.Equal(next.Images[1].Bytes(), extra.Data) {
		t.Fatal("new image did not appear as a user message after all tool results")
	}
	if !strings.Contains(hook.result, `"image_reference": true`) || !strings.Contains(hook.result, `"asset_id": "extra"`) {
		t.Fatal("hook did not receive the safe read_asset reference")
	}
	if len(records.recorded) != 1 || len(records.recorded[0].Messages) != 4 || len(records.recorded[0].Entries) != 6 {
		t.Fatal("unexpected persisted tool batch")
	}
	stored, err := memory.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{stored.Messages, records.recorded, hook.result, next.Images} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), base64.StdEncoding.EncodeToString(photo.Data)) || strings.Contains(string(encoded), base64.StdEncoding.EncodeToString(extra.Data)) || strings.Contains(string(encoded), "head_hex") || strings.Contains(string(encoded), "Image from read_asset") || strings.Contains(string(encoded), "data:image/") {
			t.Fatal("image payload or projected caption leaked from the model request")
		}
	}
	if rendering := fmt.Sprintf("%#v", next.Images); strings.Contains(rendering, "[]uint8") || strings.Contains(rendering, base64.StdEncoding.EncodeToString(extra.Data)) {
		t.Fatal("image bytes leaked through Go request formatting")
	}
	fresh := &visionModel{}
	if _, err := service.Run(t.Context(), command.Run{Session: stored, Model: fresh}); err != nil {
		t.Fatal(err)
	}
	if len(fresh.requests[0].Images) != 1 || len(fileService.requests) != 3 || fileService.requests[2].AssetID != "photo" {
		t.Fatal("historical tool response was replayed as a new image or old bytes crossed a run boundary")
	}
}
