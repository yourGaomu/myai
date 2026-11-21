package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	toolrecordsmapper "myai/core/adapter/persistence/toolrecords/mapper"
	toolexecutor "myai/core/adapter/tool/executor"
	generationcommand "myai/core/application/chat/generation/command"
	domainmessage "myai/core/domain/message"
	domaintool "myai/core/domain/tool"
	"myai/core/imageasset"
	"myai/core/port/files"
	modelport "myai/core/port/model"
	"myai/core/session"
	"myai/core/tool"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type attachImageFilesStub struct {
	files.Service
	request  files.UploadRequest
	contents []byte
	asset    files.Asset
	err      error
	uploads  int
}

func (f *attachImageFilesStub) Upload(_ context.Context, request files.UploadRequest) (files.Asset, error) {
	f.uploads++
	f.request = request
	contents, err := io.ReadAll(request.Reader)
	f.contents = contents
	return f.asset, errors.Join(err, f.err)
}

func attachmentImageBytes(t *testing.T, contentType string) []byte {
	t.Helper()
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 3))
	var data bytes.Buffer
	var err error
	if contentType == "image/jpeg" {
		err = jpeg.Encode(&data, pixels, nil)
	} else {
		err = png.Encode(&data, pixels)
	}
	if err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func writeAttachmentFixture(t *testing.T, root, path string, data []byte) string {
	t.Helper()
	path = filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func attachImageArguments(t *testing.T, path string) json.RawMessage {
	t.Helper()
	args, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	return args
}

func TestAttachImageUploadsVerifiedBytesWithoutPublishing(t *testing.T) {
	for _, tc := range []struct {
		name, path, contentType string
	}{
		{"PNG with a mislabeled extension", "results/图.jpeg", "image/png"},
		{"JPEG by absolute workspace path", "results/photo.jpg", "image/jpeg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			data := attachmentImageBytes(t, tc.contentType)
			absolute := writeAttachmentFixture(t, workspace, tc.path, data)
			path := tc.path
			if tc.contentType == "image/jpeg" {
				path, _ = filepath.EvalSymlinks(absolute)
			}
			var expires = time.Now()
			uploader := &attachImageFilesStub{asset: files.Asset{
				AssetID: "asset-image", BusinessID: "business-image", Checksum: "checksum-test",
				FileName: "unexpected-file-name", ContentType: "application/octet-stream", Size: -1,
				ShortURL: "https://example.invalid/?signature=must-not-appear", Code: "secret-code", ExpiresAt: &expires,
			}}
			output, err := NewAttachImageToolWithWorkspaceAndFiles(workspace, uploader).Call(context.Background(), attachImageArguments(t, path))
			if err != nil || output.Failed() {
				t.Fatalf("attach image failed: %+v / %v", output, err)
			}
			fileName := filepath.Base(tc.path)
			if uploader.uploads != 1 || !bytes.Equal(uploader.contents, data) || uploader.request.Size != int64(len(data)) || uploader.request.ContentType != tc.contentType || uploader.request.FileName != fileName {
				t.Fatalf("upload request did not contain the verified image: %+v", uploader.request)
			}
			var receipt attachImageResult
			if err := json.Unmarshal([]byte(output.Content), &receipt); err != nil {
				t.Fatal(err)
			}
			want := attachImageResult{AssetID: "asset-image", BusinessID: "business-image", Checksum: "checksum-test",
				Path: tc.path, FileName: fileName, ContentType: tc.contentType, Size: int64(len(data))}
			if receipt != want {
				t.Fatalf("unexpected image receipt: %+v", receipt)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(output.Content), &fields); err != nil || len(fields) != 7 {
				t.Fatalf("receipt has unexpected fields: %s / %v", output.Content, err)
			}
			wantAsset := domaintool.SharedAsset{
				AssetID: want.AssetID, BusinessID: want.BusinessID, Checksum: want.Checksum, LocalPath: want.Path,
				FileName: want.FileName, ContentType: want.ContentType, Size: want.Size,
			}
			if output.SharedAsset == nil || *output.SharedAsset != wantAsset {
				t.Fatalf("unexpected typed asset: %+v", output.SharedAsset)
			}
		})
	}
}

func TestAttachImageRejectsUnverifiedFilesBeforeUpload(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	valid := attachmentImageBytes(t, "image/png")
	outsideFile := writeAttachmentFixture(t, outside, "external.png", valid)
	writeAttachmentFixture(t, workspace, "ordinary.png", valid)
	writeAttachmentFixture(t, workspace, ".env.preview.png", valid)
	writeAttachmentFixture(t, workspace, "config/inside.png", valid)
	writeAttachmentFixture(t, workspace, "directory/nested.png", valid)
	writeAttachmentFixture(t, workspace, "fake.png", []byte("not an image"))
	writeAttachmentFixture(t, workspace, "corrupt.png", valid[:len(valid)-8])
	oversized := writeAttachmentFixture(t, workspace, "too-big.png", valid)
	if err := os.Truncate(oversized, imageasset.MaxSourceBytes+1); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"", ".env.preview.png", `config\inside.png`, ".git/../ordinary.png", "directory", "missing.png", "fake.png", "corrupt.png", "too-big.png", "../external.png", outsideFile} {
		t.Run(path, func(t *testing.T) {
			uploader := &attachImageFilesStub{asset: files.Asset{AssetID: "should-not-upload"}}
			output, err := NewAttachImageToolWithWorkspaceAndFiles(workspace, uploader).Call(context.Background(), attachImageArguments(t, path))
			if err == nil || output.SharedAsset != nil || uploader.uploads != 0 {
				t.Fatalf("unverified file reached upload: output=%+v err=%v uploads=%d", output, err, uploader.uploads)
			}
		})
	}
}

func TestAttachImageRejectsLinkedPathComponents(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	valid := attachmentImageBytes(t, "image/png")
	insideFile := writeAttachmentFixture(t, workspace, "images/inside.png", valid)
	outsideFile := writeAttachmentFixture(t, outside, "outside.png", valid)
	for _, tc := range []struct{ target, name, path string }{
		{"images/inside.png", "inside-link.png", "inside-link.png"},
		{outsideFile, "outside-link.png", "outside-link.png"},
		{filepath.Dir(insideFile), "inside-dir-link", "inside-dir-link/inside.png"},
		{outside, "outside-dir-link", "outside-dir-link/outside.png"},
		{outside, "parent-alias", "parent-alias/../images/inside.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Symlink(filepath.FromSlash(tc.target), filepath.Join(workspace, tc.name)); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			uploader := &attachImageFilesStub{}
			output, err := NewAttachImageToolWithWorkspaceAndFiles(workspace, uploader).Call(context.Background(), attachImageArguments(t, tc.path))
			if err == nil || output.SharedAsset != nil || uploader.uploads != 0 {
				t.Fatalf("linked path reached upload: output=%+v err=%v uploads=%d", output, err, uploader.uploads)
			}
		})
	}
}

func TestAttachImageUsesOnlyTheCurrentSandboxMirror(t *testing.T) {
	host := t.TempDir()
	mirror := filepath.Join(host, ".myai", "subagent-snapshots", "child", "workspace")
	valid := attachmentImageBytes(t, "image/png")
	hostFile := writeAttachmentFixture(t, host, "image.png", valid)
	writeAttachmentFixture(t, mirror, "report.png", valid)
	uploader := &attachImageFilesStub{asset: files.Asset{AssetID: "asset-mirror"}}
	attachment := NewAttachImageToolWithWorkspaceAndFiles(host, uploader)
	execution := toolruntime.Execution{WorkspaceRoot: mirror, SandboxID: "remote-sandbox"}
	ctx := toolruntime.WithExecution(context.Background(), execution)
	for _, path := range []string{"image.png", hostFile} {
		if output, err := attachment.Call(ctx, attachImageArguments(t, path)); err == nil || output.SharedAsset != nil {
			t.Fatalf("host path reached attachment: %+v / %v", output, err)
		}
	}
	missingMirror := toolruntime.WithExecution(context.Background(), toolruntime.Execution{SandboxID: execution.SandboxID})
	if output, err := attachment.Call(missingMirror, attachImageArguments(t, "image.png")); err == nil || output.SharedAsset != nil || uploader.uploads != 0 {
		t.Fatalf("missing mirror fell back to the host: %+v / %v", output, err)
	}
	output, err := attachment.Call(ctx, attachImageArguments(t, "report.png"))
	if err != nil || output.SharedAsset == nil || output.SharedAsset.LocalPath != "report.png" || uploader.uploads != 1 {
		t.Fatalf("mirror attachment failed: %+v / %v", output, err)
	}
}

func TestAttachImageFailsWithoutAnUploadReceipt(t *testing.T) {
	workspace := t.TempDir()
	writeAttachmentFixture(t, workspace, "image.png", attachmentImageBytes(t, "image/png"))
	for _, err := range []error{nil, errors.New("upload failed")} {
		backend := &attachImageFilesStub{err: err}
		output, gotErr := NewAttachImageToolWithWorkspaceAndFiles(workspace, backend).Call(context.Background(), attachImageArguments(t, "image.png"))
		if gotErr == nil || output.SharedAsset != nil || backend.uploads != 1 {
			t.Fatalf("missing receipt treated as an attachment: %+v / %v", output, gotErr)
		}
	}
}

func TestAttachImageRequiresFreshConsentAndPreservesBatchIdentity(t *testing.T) {
	workspace := t.TempDir()
	writeAttachmentFixture(t, workspace, "image.png", attachmentImageBytes(t, "image/png"))
	uploader := &attachImageFilesStub{asset: files.Asset{AssetID: "asset-uploaded"}}
	attachment := NewAttachImageToolWithWorkspaceAndFiles(workspace, uploader)
	registered := tool.NewRegisterTools()
	registered.Register(attachment)
	executor := toolexecutor.Executor{Registry: registered}
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
