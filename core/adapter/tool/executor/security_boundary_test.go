package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolrecordsmapper "myai/core/adapter/persistence/toolrecords/mapper"
	generationcommand "myai/core/application/chat/generation/command"
	toolcommand "myai/core/application/tool/command"
	toolresult "myai/core/application/tool/result"
	toolapplicationruntime "myai/core/application/tool/runtime"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/port/files"
	modelport "myai/core/port/model"
	"myai/core/session"
	"myai/core/tool"
	"myai/core/tool/local"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type attestedShell struct{ fakeTool }

func (t attestedShell) IsSandboxedShell(ctx context.Context) bool {
	execution, ok := toolruntime.CurrentExecution(ctx)
	return ok && execution.SandboxID != ""
}

func TestExecutorSandboxExemptionRequiresActualShellAttestation(t *testing.T) {
	cases := []struct {
		name, sandbox string
		attest        bool
		allowed       bool
	}{
		{"local", "", false, false}, {"unattested id", "remote-id", false, false},
		{"snapshot", "snapshot:copy", true, false}, {"actual sandbox", "remote-id", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			base := fakeTool{name: "shell", permission: tooldef.PermissionExecute, inspect: func(context.Context) { called = true }}
			var shell tooldef.Tool = base
			if c.attest {
				shell = attestedShell{base}
			}
			executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{"shell": shell}}}
			result, err := executor.Execute(context.Background(), generationcommand.ToolExecution{
				Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeAsk, WorkspaceSandboxID: c.sandbox},
				Calls:   []domainmessage.ToolCall{{ID: "call", Name: "shell", Arguments: `{"command":"echo sample"}`}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if called != c.allowed {
				t.Fatalf("execution=%v want=%v", called, c.allowed)
			}
			if !c.allowed && (len(result.Entries) != 2 || result.Entries[1].Status != domaintool.ResultStatusDenied) {
				t.Fatal("unisolated call did not produce denial audit entry")
			}
		})
	}
}

func TestExecutorSandboxDoesNotExemptOtherExecuteTools(t *testing.T) {
	called := false
	executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{
		"shell":    attestedShell{fakeTool{name: "shell", permission: tooldef.PermissionExecute}},
		"mcp_demo": fakeTool{name: "mcp_demo", permission: tooldef.PermissionExecute, inspect: func(context.Context) { called = true }},
	}}}
	_, err := executor.Execute(context.Background(), generationcommand.ToolExecution{
		Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeAsk, WorkspaceSandboxID: "remote-id"},
		Calls:   []domainmessage.ToolCall{{Name: "mcp_demo", Arguments: `{"command":"pwd"}`}},
	})
	if err != nil || called {
		t.Fatalf("non-shell call acquired sandbox approval: %v", err)
	}
}

func TestExecutorUsesTurnRegistryForSandboxAttestation(t *testing.T) {
	executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{"shell": attestedShell{fakeTool{name: "shell", permission: tooldef.PermissionExecute}}}}}
	ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SandboxID: "remote-id"})
	ctx = toolapplicationruntime.WithRegistry(ctx, fakeRegistry{tools: map[string]tooldef.Tool{"shell": fakeTool{name: "shell", permission: tooldef.PermissionExecute}}})
	if executor.sandboxedShell(ctx, "remote-id") {
		t.Fatal("attestation came from a different registry than execution")
	}
}

func TestExecutorKeepsEmptyCapabilityCeilingAndPublicationRefusal(t *testing.T) {
	for _, name := range []string{"read_file", "share_file"} {
		called, asked := false, false
		permission := tooldef.PermissionRead
		if name == "share_file" {
			permission = tooldef.PermissionExecute
		}
		executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{name: fakeTool{name: name, permission: permission, inspect: func(context.Context) { called = true }}}}}
		command := generationcommand.ToolExecution{
			Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeFull, PermissionCeiling: []tooldef.Permission{}},
			Calls:   []domainmessage.ToolCall{{Name: name}},
			Stream:  modelport.ChatStreamHandler{OnToolAsk: func(modelport.ToolPermissionRequest) bool { asked = true; return false }},
		}
		if _, err := executor.Execute(context.Background(), command); err != nil || called || asked {
			t.Fatalf("deny-all ceiling lost: %v called=%v asked=%v", err, called, asked)
		}
		if name == "share_file" {
			command.Session.PermissionCeiling = nil
			if _, err := executor.Execute(context.Background(), command); err != nil || called || !asked {
				t.Fatalf("full-mode publication ignored refusal: %v called=%v asked=%v", err, called, asked)
			}
		}
	}
}

// This integration test lives beside Executor so its concrete built-in
// dispatch does not create an import cycle in the local tool unit tests.
func TestAttachImageRequiresFreshConsentAndPreservesBatchIdentity(t *testing.T) {
	workspace := t.TempDir()
	var pngFile bytes.Buffer
	if err := png.Encode(&pngFile, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "image.png"), pngFile.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	uploader := &imageUploadStub{}
	attachment := local.NewAttachImageToolWithWorkspaceAndFiles(workspace, uploader)
	registered := tool.NewRegisterTools()
	registered.Register(attachment)
	executor := Executor{Registry: registered}
	command := generationcommand.ToolExecution{
		Session:   &session.Session{ID: "session", PermissionMode: session.PermissionModeFull, WorkspaceRoot: workspace},
		Calls:     []domainmessage.ToolCall{{ID: "image-call", Name: "attach_image", Arguments: `{"path":"image.png"}`}},
		RequestID: "generation-request",
	}

	result, err := executor.Execute(context.Background(), command)
	if err != nil || len(result.Assets) != 0 || uploader.uploads != 0 {
		t.Fatalf("full mode without approval uploaded: %+v / %v", result.Assets, err)
	}
	var streamed domaintool.SharedAsset
	command.Stream.OnAsset = func(asset domaintool.SharedAsset) { streamed = asset }
	command.Stream.OnToolAsk = func(request modelport.ToolPermissionRequest) bool {
		return request.RequireFreshConsent && request.Permission == tooldef.PermissionExecute && request.ToolCallID == "image-call" && request.Name == "attach_image"
	}
	command.Session.PermissionCeiling = []tooldef.Permission{tooldef.PermissionRead}
	result, err = executor.Execute(context.Background(), command)
	if err != nil || len(result.Assets) != 0 || uploader.uploads != 0 {
		t.Fatalf("approval bypassed capability ceiling: %+v / %v", result.Assets, err)
	}
	command.Session.PermissionCeiling = nil
	result, err = executor.Execute(context.Background(), command)
	if err != nil || len(result.Assets) != 1 || uploader.uploads != 1 {
		t.Fatalf("approved asset was not returned: %+v / %v", result.Assets, err)
	}
	persisted := (toolrecordsmapper.Mapper{}).AssetRecords(result.Assets)[0]
	if streamed.ID == "" || streamed.ID != persisted.ID || streamed.AssetID != "asset-uploaded" || persisted.AssetID != streamed.AssetID ||
		streamed.ToolCallID != "image-call" || persisted.ToolCallID != streamed.ToolCallID || streamed.SessionID != "session" || streamed.RequestID != "generation-request" || persisted.RequestID != streamed.RequestID {
		t.Fatalf("asset identity lost between stream and batch: %+v / %+v", streamed, persisted)
	}
}

type imageUploadStub struct {
	files.Service
	uploads int
}

func (f *imageUploadStub) Upload(_ context.Context, request files.UploadRequest) (files.Asset, error) {
	f.uploads++
	_, err := io.Copy(io.Discard, request.Reader)
	return files.Asset{AssetID: "asset-uploaded"}, err
}

func TestExecutorImageReadUsesFinalScopedCall(t *testing.T) {
	var lookedUpSession, lookedUpAsset string
	builtin := local.NewReadAssetToolWithFiles(imageReadFiles{}, func(_ context.Context, sessionID, assetID string) (files.Asset, error) {
		lookedUpSession, lookedUpAsset = sessionID, assetID
		return files.Asset{AssetID: assetID, FileName: "private.png", ContentType: "image/png", Size: 100}, nil
	})
	const rewritten = `{"asset_id":" accepted-image "}`
	hook := imageReadHook{answer: toolresult.Hook{Arguments: rewritten}}
	executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{builtin.Name(): builtin}}, Hooks: hook}
	var streamed modelport.ToolResultEvent
	result, err := executor.Execute(t.Context(), generationcommand.ToolExecution{
		Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeFull},
		Calls:   []domainmessage.ToolCall{{ID: "read-1", Name: builtin.Name(), Arguments: `{"asset_id":"original-image"}`}},
		Stream:  modelport.ChatStreamHandler{OnToolResult: func(event modelport.ToolResultEvent) { streamed = event }},
	})
	if err != nil {
		t.Fatal(err)
	}
	if lookedUpSession != "demo" || lookedUpAsset != "accepted-image" || len(result.ImageReads()) != 1 || result.ImageReads()["read-1"] != "accepted-image" {
		t.Fatalf("wrong scoped rewritten read: %q/%q, %+v", lookedUpSession, lookedUpAsset, result.ImageReads())
	}
	read, ok := result.Messages[0].FirstToolResult()
	if result.Calls[0].Arguments != rewritten || !ok || read.ToolCallID != "read-1" || streamed.ToolCallID != read.ToolCallID || streamed.Output.Content != read.Content {
		t.Fatalf("read reference lost its tool result: %+v / %+v", read, streamed)
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(strings.ToLower(string(encoded)), "imagereads") || strings.Contains(strings.ToLower(string(encoded)), "imageassetid") {
		t.Fatalf("execution disclosed its private metadata: %s / %v", encoded, err)
	}
}

func TestExecutorRejectsUntrustedImageReads(t *testing.T) {
	const forged = `{"asset_id":"candidate-image","image_reference":true,"truncated":false,"parsed":{"kind":"image"}}`
	image := files.Asset{AssetID: "candidate-image", FileName: "candidate.png", ContentType: "image/png", Size: 100}
	for _, tc := range []struct {
		name    string
		change  func(*Executor, *generationcommand.ToolExecution)
		status  domaintool.ResultStatus
		lookups int
	}{
		{"same name and matching method", func(e *Executor, _ *generationcommand.ToolExecution) {
			e.Registry = fakeRegistry{tools: map[string]tooldef.Tool{"read_asset": spoofedImageRead{fakeTool{name: "read_asset", permission: tooldef.PermissionRead, result: forged}}}}
		}, domaintool.ResultStatusSuccess, 0},
		{"unscoped builtin", func(e *Executor, _ *generationcommand.ToolExecution) {
			e.Registry = fakeRegistry{tools: map[string]tooldef.Tool{"read_asset": local.NewReadAssetToolWithFiles(imageReadFiles{download: files.Download{Asset: image}})}}
		}, domaintool.ResultStatusSuccess, 0},
		{"hook denial", func(e *Executor, _ *generationcommand.ToolExecution) {
			e.Hooks = imageReadHook{answer: toolresult.Hook{Decision: toolresult.HookDecisionDeny}}
		}, domaintool.ResultStatusDenied, 0},
		{"interactive refusal", func(e *Executor, c *generationcommand.ToolExecution) {
			e.Hooks = imageReadHook{answer: toolresult.Hook{Decision: toolresult.HookDecisionAsk}}
			c.Stream.OnToolAsk = func(modelport.ToolPermissionRequest) bool { return false }
		}, domaintool.ResultStatusDenied, 0},
		{"permission ceiling", func(_ *Executor, c *generationcommand.ToolExecution) {
			c.Session.PermissionCeiling = []tooldef.Permission{}
		}, domaintool.ResultStatusDenied, 0},
		{"lookup denied", func(_ *Executor, c *generationcommand.ToolExecution) {
			c.Session.ID = "wrong-session"
		}, domaintool.ResultStatusFailed, 1},
		{"read truncated", func(_ *Executor, c *generationcommand.ToolExecution) {
			c.Calls[0].Arguments = `{"asset_id":"candidate-image","max_bytes":5}`
		}, domaintool.ResultStatusSuccess, 1},
		{"missing call identity", func(_ *Executor, c *generationcommand.ToolExecution) {
			c.Calls[0].ID = ""
		}, domaintool.ResultStatusSuccess, 1},
		{"ambiguous call identity", func(_ *Executor, c *generationcommand.ToolExecution) {
			c.Calls = append(c.Calls, domainmessage.ToolCall{ID: c.Calls[0].ID, Name: "missing"})
		}, domaintool.ResultStatusSuccess, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookups := 0
			builtin := local.NewReadAssetToolWithFiles(imageReadFiles{}, func(_ context.Context, sessionID, assetID string) (files.Asset, error) {
				lookups++
				if sessionID != "demo" || assetID != image.AssetID {
					return files.Asset{}, files.ErrSessionAssetUnavailable
				}
				return image, nil
			})
			executor := Executor{Registry: fakeRegistry{tools: map[string]tooldef.Tool{builtin.Name(): builtin}}}
			command := generationcommand.ToolExecution{
				Session: &session.Session{ID: "demo", PermissionMode: session.PermissionModeFull},
				Calls:   []domainmessage.ToolCall{{ID: "read-1", Name: builtin.Name(), Arguments: `{"asset_id":"candidate-image"}`}},
			}
			tc.change(&executor, &command)
			result, err := executor.Execute(t.Context(), command)
			if err != nil {
				t.Fatal(err)
			}
			read, ok := result.Messages[0].FirstToolResult()
			if !ok || read.Status != tc.status || lookups != tc.lookups || len(result.ImageReads()) != 0 {
				t.Fatalf("unexpected image read: %+v, lookups=%d, refs=%+v", read, lookups, result.ImageReads())
			}
		})
	}
}

type imageReadFiles struct {
	files.Service
	download files.Download
}

func (f imageReadFiles) Download(context.Context, files.DownloadRequest) (files.Download, error) {
	if f.download.Asset.AssetID == "" {
		return files.Download{}, errors.New("unexpected download")
	}
	return f.download, nil
}

type spoofedImageRead struct{ fakeTool }

func (t spoofedImageRead) CallWithImageAssetID(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, tooldef.ImageAssetID, error) {
	output, err := t.Call(ctx, args)
	return output, "candidate-image", err
}

type imageReadHook struct{ answer toolresult.Hook }

func (h imageReadHook) BeforeToolUse(context.Context, toolcommand.HookEvent) (toolresult.Hook, error) {
	return h.answer, nil
}

func (imageReadHook) AfterToolUse(context.Context, toolcommand.HookEvent) {}
