package opensandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	domainsandbox "myai/core/domain/sandbox"
)

func TestLocalSynchronizationFiltersSharedPolicyAndProtectsLegacyEntries(t *testing.T) {
	root := t.TempDir()
	allowed := []string{"docs/report.txt", "src/config.go", "src/configuration.go", "src/main.go"}
	excluded := []string{".env", "nested/.ENV.test", "config/service.go", "application.yaml", "relay.yml", "privatekeys/signing.txt", "keys/privatekey.pem", ".myai/history.db", "plugins/demo/main.go"}
	for _, path := range append(append([]string{}, allowed...), excluded...) {
		writeWorkspaceFile(t, root, path, "temporary fixture\n")
	}
	current, err := scanLocal(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if paths := sortedPaths(current); !reflect.DeepEqual(paths, allowed) {
		t.Fatalf("scanned paths = %#v, want %#v", paths, allowed)
	}
	remote := &observedRemote{fakeRemoteWorkspace: &fakeRemoteWorkspace{id: "sandbox", files: make(map[string]domainsandbox.File)}}
	previous := make(map[string]fileState)
	for _, path := range excluded {
		previous[path] = fileState{Path: path, Hash: "legacy"}
		// Supply excluded current entries too: upload must defend independently
		// even if handed an old or unfiltered manifest by its caller.
		current[path] = fileState{Path: path, Hash: "changed"}
	}
	if err := uploadChanged(context.Background(), remote, root, previous, current); err != nil {
		t.Fatal(err)
	}
	if remote.commands != 0 {
		t.Fatalf("excluded legacy entries generated deletion commands: %d", remote.commands)
	}
	paths := make([]string, 0, len(remote.files))
	for path := range remote.files {
		paths = append(paths, strings.TrimPrefix(path, "/workspace/"))
	}
	sort.Strings(paths)
	if !reflect.DeepEqual(paths, allowed) {
		t.Fatalf("uploaded paths = %#v, want %#v", paths, allowed)
	}
	if err := uploadChanged(context.Background(), remote, root, previous, nil); err != nil || remote.commands != 0 {
		t.Fatalf("legacy sensitive paths must not be deleted remotely: commands=%d, %v", remote.commands, err)
	}
}

func TestSynchronizeDownFiltersBeforeHashAndDownloadAndPreservesLegacyFiles(t *testing.T) {
	root := t.TempDir()
	excluded := []string{".env", "application.yaml", "config/service.json", "keys/privatekey.pem", "../outside.txt", `nested\escape.txt`}
	remote := &observedRemote{fakeRemoteWorkspace: &fakeRemoteWorkspace{id: "sandbox", files: make(map[string]domainsandbox.File)}}
	previous := make(map[string]fileState)
	for _, path := range excluded {
		if validSynchronizedPath(path) {
			writeWorkspaceFile(t, root, path, "local sentinel\n")
		}
		previous[path] = fileState{Path: path, Hash: "legacy"}
		remote.files["/workspace/"+path] = domainsandbox.File{Path: "/workspace/" + path, Content: []byte("remote sentinel\n"), Mode: 0o600}
	}
	writeWorkspaceFile(t, root, "delete.txt", "delete me\n")
	previous["delete.txt"] = fileState{Path: "delete.txt", Hash: "old"}
	remote.files["/workspace/src/config.go"] = domainsandbox.File{Path: "/workspace/src/config.go", Content: []byte("package src\n"), Mode: 0o644}
	if err := synchronizeDown(context.Background(), remote, root, previous); err != nil {
		t.Fatal(err)
	}
	assertWorkspaceFile(t, root, "src/config.go", "package src\n")
	for _, path := range excluded {
		if validSynchronizedPath(path) {
			assertWorkspaceFile(t, root, path, "local sentinel\n")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "delete.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary remote deletion was lost: %v", err)
	}
	if !reflect.DeepEqual(remote.downloadedFiles, []string{"/workspace/src/config.go"}) {
		t.Fatalf("excluded contents were downloaded: %#v", remote.downloadedFiles)
	}
	if len(remote.hashAllowed) != 1 || !reflect.DeepEqual(remote.hashAllowed[0], []string{"src/config.go"}) {
		t.Fatalf("excluded names were sent to remote hashing: %#v", remote.hashAllowed)
	}
}

func TestSynchronizedPathsRejectTraversalAndProtectedWrites(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"../outside.txt", "/outside.txt", "a/../../outside.txt", `a\outside.txt`, "file.txt:stream", "dir./file.txt", "dir /file.txt", ".env", "config/file.json", "keys/privatekey.pem"} {
		if _, err := safeLocalPath(root, path); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
		if err := writeLocalFile(root, fileState{Path: path, Mode: 0o600}, []byte("should not be written")); err == nil {
			t.Errorf("unsafe write accepted: %q", path)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "existing-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalFile(root, fileState{Path: "existing-dir", Mode: 0o600}, []byte("must not replace directory")); err == nil {
		t.Fatal("synchronization replaced an existing directory")
	}
	info, err := os.Stat(filepath.Join(root, "existing-dir"))
	if err != nil || !info.IsDir() {
		t.Fatalf("rejected write removed the directory: %v, %v", info, err)
	}
}

func TestLocalSynchronizationRejectsSymlinkAncestors(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeWorkspaceFile(t, outside, "sentinel.txt", "outside sentinel\n")
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Skipf("directory symlink creation unavailable: %v", err)
	}
	scanned, err := scanLocal(context.Background(), root)
	if err != nil || len(scanned) != 0 {
		t.Fatalf("scan followed escaping symlink: %#v, %v", scanned, err)
	}
	remote := &fakeRemoteWorkspace{id: "sandbox", files: make(map[string]domainsandbox.File)}
	if err := uploadChanged(context.Background(), remote, root, nil, map[string]fileState{"alias/sentinel.txt": {Path: "alias/sentinel.txt"}}); err == nil {
		t.Fatal("upload followed an escaping symlink")
	}
	if err := writeLocalFile(root, fileState{Path: "alias/sentinel.txt", Mode: 0o600}, []byte("changed")); err == nil {
		t.Fatal("write followed an escaping symlink")
	}
	if err := removeLocalFile(root, "alias/sentinel.txt"); err == nil {
		t.Fatal("delete followed an escaping symlink")
	}
	assertWorkspaceFile(t, outside, "sentinel.txt", "outside sentinel\n")
}

// Run the actual remote scripts on POSIX, where dir_fd and O_NOFOLLOW exist.
// Windows exercises its local junction protection in files_windows_test.go.
func TestRemoteScriptsRejectLinksAndOperateOnRegularFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("remote sandbox scripts require POSIX dir_fd and O_NOFOLLOW")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	root, outside := t.TempDir(), t.TempDir()
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	writeWorkspaceFile(t, root, "src/config.go", "package src\n")
	writeWorkspaceFile(t, root, "config/secret.txt", "temporary sensitive fixture\n")
	writeWorkspaceFile(t, outside, "sentinel.txt", "outside sentinel\n")
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	run := func(script string) error {
		t.Helper()
		output, err := exec.Command(python, "-I", "-c", script, root, manifest).CombinedOutput()
		if err != nil {
			t.Logf("remote script output: %s", output)
		}
		return err
	}
	if err := run(remoteManifestScript); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var files []fileState
	if err := json.Unmarshal(content, &files); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Path, "alias/") {
			t.Fatalf("remote walk traversed a symlink: %#v", files)
		}
	}
	if err := run(remoteOpenScript + "\nallowed=['src/config.go']\n" + remoteHashScript); err != nil {
		t.Fatal(err)
	}
	if err := run(remoteOpenScript + "\nallowed=['alias/sentinel.txt']\n" + remoteHashScript); err == nil {
		t.Fatal("remote hasher followed a linked directory")
	}
	if err := os.Symlink(filepath.Join(outside, "sentinel.txt"), filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	if err := run(remoteOpenScript + "\nallowed=['alias.txt']\n" + remoteHashScript); err == nil {
		t.Fatal("remote hasher followed a linked file")
	}
}

type observedRemote struct {
	*fakeRemoteWorkspace
	commands        int
	downloadedFiles []string
	hashAllowed     [][]string
}

func (r *observedRemote) Run(ctx context.Context, request domainsandbox.CommandRequest) (domainsandbox.CommandResult, error) {
	r.commands++
	if !strings.HasPrefix(request.Command, "python -I -c") || request.WorkDir != "/tmp" {
		return domainsandbox.CommandResult{}, errors.New("sync scripts must use isolated Python outside the workspace")
	}
	start := strings.Index(request.Command, "b64decode('")
	if start < 0 {
		return domainsandbox.CommandResult{}, errors.New("script missing")
	}
	start += len("b64decode('")
	end := strings.Index(request.Command[start:], "')")
	if end < 0 {
		return domainsandbox.CommandResult{}, errors.New("encoded script missing")
	}
	script, err := base64.StdEncoding.DecodeString(request.Command[start : start+end])
	if err != nil {
		return domainsandbox.CommandResult{}, err
	}
	const allowedMarker = "\nallowed=json.loads("
	if position := strings.Index(string(script), allowedMarker); position >= 0 {
		literal := strings.SplitN(string(script)[position+len(allowedMarker):], "\n", 2)[0]
		literal = strings.TrimSuffix(literal, ")")
		var encodedPaths string
		if err := json.Unmarshal([]byte(literal), &encodedPaths); err != nil {
			return domainsandbox.CommandResult{}, err
		}
		var paths []string
		if err := json.Unmarshal([]byte(encodedPaths), &paths); err != nil {
			return domainsandbox.CommandResult{}, err
		}
		r.hashAllowed = append(r.hashAllowed, paths)
	}
	return r.fakeRemoteWorkspace.Run(ctx, request)
}

func (r *observedRemote) DownloadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if path != remoteManifestPath {
		r.downloadedFiles = append(r.downloadedFiles, path)
	}
	return r.fakeRemoteWorkspace.DownloadFile(ctx, path, maxBytes)
}
