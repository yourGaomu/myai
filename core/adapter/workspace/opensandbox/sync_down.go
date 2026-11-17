package opensandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	domainsandbox "myai/core/domain/sandbox"
)

const remoteManifestPath = "/tmp/myai-workspace-manifest.json"

// Enumerate names and metadata only. Go applies pathpolicy before a second
// command opens any file contents, so the remote code needs no credential list.
const remoteManifestScript = `import json,os,stat,sys
root=sys.argv[1]
manifest=sys.argv[2]
items=[]
rootfd=os.open(root,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
try:
    for base,dirs,files,basefd in os.fwalk('.',dir_fd=rootfd,follow_symlinks=False):
        dirs[:]=[d for d in dirs if stat.S_ISDIR(os.stat(d,dir_fd=basefd,follow_symlinks=False).st_mode)]
        for name in files:
            st=os.stat(name,dir_fd=basefd,follow_symlinks=False)
            if not stat.S_ISREG(st.st_mode): continue
            rel=os.path.normpath(os.path.join(base,name)).replace(os.sep,'/')
            items.append({'path':rel,'size':st.st_size,'mode':stat.S_IMODE(st.st_mode)})
finally:
    os.close(rootfd)
fd=os.open(manifest,os.O_WRONLY|os.O_CREAT|os.O_TRUNC|os.O_NOFOLLOW,0o600)
with os.fdopen(fd,'w',encoding='utf-8') as f: json.dump(items,f)
`

// Resolve each path beneath a directory descriptor without following links in
// any component. O_NONBLOCK avoids blocking on a file swapped for a FIFO.
const remoteOpenScript = `import hashlib,json,os,stat,sys
root=sys.argv[1]
manifest=sys.argv[2]
def open_regular(rootfd,relative):
    parts=relative.split('/')
    if not parts or any(p in ('','.','..') or '\\' in p or ':' in p for p in parts):
        raise ValueError('invalid workspace-relative path')
    parent=os.dup(rootfd)
    try:
        for part in parts[:-1]:
            child=os.open(part,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=parent)
            os.close(parent)
            parent=child
        fd=os.open(parts[-1],os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=parent)
        if not stat.S_ISREG(os.fstat(fd).st_mode):
            os.close(fd)
            raise ValueError('workspace path is not a regular file')
        return fd
    finally:
        os.close(parent)
`

const remoteHashScript = `items=[]
rootfd=os.open(root,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
try:
    for relative in allowed:
        fd=open_regular(rootfd,relative)
        with os.fdopen(fd,'rb') as f:
            st=os.fstat(f.fileno())
            h=hashlib.sha256()
            for block in iter(lambda:f.read(131072),b''): h.update(block)
        items.append({'path':relative,'hash':h.hexdigest(),'size':st.st_size,'mode':stat.S_IMODE(st.st_mode)})
finally:
    os.close(rootfd)
fd=os.open(manifest,os.O_WRONLY|os.O_CREAT|os.O_TRUNC|os.O_NOFOLLOW,0o600)
with os.fdopen(fd,'w',encoding='utf-8') as f: json.dump(items,f)
`

func synchronizeDown(ctx context.Context, remote remoteWorkspace, localRoot string, previous map[string]fileState) error {
	files, err := remoteManifest(ctx, remote, remoteManifestScript)
	if err != nil {
		return err
	}
	allowed := make(map[string]fileState, len(files))
	for _, file := range files {
		if skipped(file.Path) {
			continue
		}
		if file.Size < 0 || file.Size > maxSynchronizedFileBytes {
			return fmt.Errorf("remote file %s exceeds synchronization limit of %d bytes", file.Path, maxSynchronizedFileBytes)
		}
		allowed[file.Path] = file
	}
	current := make(map[string]fileState, len(allowed))
	if len(allowed) > 0 {
		paths, err := json.Marshal(sortedPaths(allowed))
		if err != nil {
			return err
		}
		script := remoteOpenScript + "\nallowed=json.loads(" + fmt.Sprintf("%q", string(paths)) + ")\n" + remoteHashScript
		files, err = remoteManifest(ctx, remote, script)
		if err != nil {
			return err
		}
		for _, file := range files {
			if _, ok := allowed[file.Path]; ok && !skipped(file.Path) {
				current[file.Path] = file
			}
		}
	}
	for path := range previous {
		if skipped(path) {
			continue
		}
		if _, exists := current[path]; exists {
			continue
		}
		if err := removeLocalFile(localRoot, path); err != nil {
			return err
		}
	}
	for _, path := range sortedPaths(current) {
		file := current[path]
		if before, exists := previous[path]; exists && before.Hash == file.Hash && before.Size == file.Size {
			continue
		}
		if file.Size < 0 || file.Size > maxSynchronizedFileBytes {
			return fmt.Errorf("remote file %s exceeds synchronization limit of %d bytes", path, maxSynchronizedFileBytes)
		}
		if _, err := safeLocalPath(localRoot, path); err != nil {
			return err
		}
		content, err := remote.DownloadFile(ctx, "/workspace/"+path, maxSynchronizedFileBytes)
		if err != nil {
			return err
		}
		if int64(len(content)) > maxSynchronizedFileBytes {
			return fmt.Errorf("remote file %s exceeds synchronization limit of %d bytes", path, maxSynchronizedFileBytes)
		}
		if err := writeLocalFile(localRoot, file, content); err != nil {
			return err
		}
	}
	return nil
}

func remoteManifest(ctx context.Context, remote remoteWorkspace, script string) ([]fileState, error) {
	result, err := remote.Run(ctx, domainsandbox.CommandRequest{
		Command: remotePythonCommand(script), WorkDir: "/tmp",
	})
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("build OpenSandbox workspace manifest failed: %s", strings.TrimSpace(result.Stderr))
	}
	content, err := remote.DownloadFile(ctx, remoteManifestPath, 8*1024*1024)
	if err != nil {
		return nil, err
	}
	var files []fileState
	if err := json.Unmarshal(content, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func remotePythonCommand(script string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	return "python -I -c \"import base64;exec(base64.b64decode('" + encoded + "'))\" /workspace " + remoteManifestPath
}

func deleteRemoteFiles(ctx context.Context, remote remoteWorkspace, paths []string) error {
	allowed := make([]string, 0, len(paths))
	for _, path := range paths {
		if !skipped(path) {
			allowed = append(allowed, path)
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	content, err := json.Marshal(allowed)
	if err != nil {
		return err
	}
	// Open all parent components without following symlinks before unlinking
	// the regular child. A symlink parent cannot redirect a remote deletion.
	script := remoteOpenScript + "\nallowed=json.loads(" + fmt.Sprintf("%q", string(content)) + ")\n" + `rootfd=os.open(root,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
try:
    for relative in allowed:
        parent=os.dup(rootfd)
        try:
            parts=relative.split('/')
            for part in parts[:-1]:
                child=os.open(part,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=parent)
                os.close(parent)
                parent=child
            st=os.stat(parts[-1],dir_fd=parent,follow_symlinks=False)
            if not stat.S_ISREG(st.st_mode): raise ValueError('workspace path is not a regular file')
            os.unlink(parts[-1],dir_fd=parent)
        except FileNotFoundError:
            pass
        finally:
            os.close(parent)
finally:
    os.close(rootfd)
`
	result, err := remote.Run(ctx, domainsandbox.CommandRequest{
		Command: remotePythonCommand(script), WorkDir: "/tmp",
	})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("delete stale OpenSandbox files failed: %s", strings.TrimSpace(result.Stderr))
	}
	return nil
}
