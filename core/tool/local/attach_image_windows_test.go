package local

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"myai/core/port/files"
	toolruntime "myai/core/tool/runtimecontext"
)

func TestAttachImageRejectsWindowsJunctions(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	data := attachmentImageBytes(t, "image/png")
	inside := filepath.Dir(writeAttachmentFixture(t, workspace, "images/artifact.png", data))
	writeAttachmentFixture(t, outside, "artifact.png", data)

	for _, fixture := range []struct{ name, target string }{
		{"inside-junction", inside}, {"outside-junction", outside},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			junction := filepath.Join(workspace, fixture.name)
			if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, fixture.target).CombinedOutput(); err != nil {
				t.Skipf("junction creation unavailable: %v (%s)", err, output)
			}
			uploader := &attachImageFilesStub{asset: files.Asset{AssetID: "unexpected"}}
			attachment := NewAttachImageToolWithWorkspaceAndFiles(workspace, uploader)
			if _, err := attachment.Call(context.Background(), attachImageArguments(t, fixture.name+"/artifact.png")); err == nil {
				t.Fatal("junction reached the upload tool")
			}
			ctx := toolruntime.WithExecution(context.Background(), toolruntime.Execution{WorkspaceRoot: junction, SandboxID: "remote-sandbox"})
			if _, err := attachment.Call(ctx, attachImageArguments(t, "artifact.png")); err == nil || uploader.uploads != 0 {
				t.Fatalf("linked sandbox root reached the upload tool: uploads=%d, err=%v", uploader.uploads, err)
			}
		})
	}
}
