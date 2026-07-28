package opensandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	domainsandbox "myai/core/domain/sandbox"
)

const remoteManifestPath = "/tmp/myai-workspace-manifest.json"

const remoteManifestScript = `import hashlib,json,os,stat
root='/workspace'
ignored={'.git','.idea','.expo','.next','.cache','node_modules','dist','build'}
items=[]
for base,dirs,files in os.walk(root):
    dirs[:]=[d for d in dirs if d not in ignored]
    for name in files:
        path=os.path.join(base,name)
        if os.path.islink(path) or not os.path.isfile(path): continue
        h=hashlib.sha256()
        with open(path,'rb') as f:
            for block in iter(lambda:f.read(131072),b''): h.update(block)
        rel=os.path.relpath(path,root).replace(os.sep,'/')
        st=os.stat(path)
        items.append({'path':rel,'hash':h.hexdigest(),'size':st.st_size,'mode':stat.S_IMODE(st.st_mode)})
with open('/tmp/myai-workspace-manifest.json','w',encoding='utf-8') as f: json.dump(items,f)
`

func synchronizeDown(ctx context.Context, remote remoteWorkspace, localRoot string, previous map[string]fileState) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(remoteManifestScript))
	result, err := remote.Run(ctx, domainsandbox.CommandRequest{
		Command: "python -c \"import base64;exec(base64.b64decode('" + encoded + "'))\"",
		WorkDir: "/workspace",
	})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("build OpenSandbox workspace manifest failed: %s", strings.TrimSpace(result.Stderr))
	}
	content, err := remote.DownloadFile(ctx, remoteManifestPath, 8*1024*1024)
	if err != nil {
		return err
	}
	var files []fileState
	if err := json.Unmarshal(content, &files); err != nil {
		return err
	}
	current := make(map[string]fileState, len(files))
	for _, file := range files {
		if file.Path == "" || skipped(file.Path) {
			continue
		}
		current[file.Path] = file
	}
	for path := range previous {
		if _, exists := current[path]; exists {
			continue
		}
		localPath, err := safeLocalPath(localRoot, path)
		if err != nil {
			return err
		}
		if err := os.Remove(localPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, path := range sortedPaths(current) {
		file := current[path]
		if before, exists := previous[path]; exists && before.Hash == file.Hash && before.Size == file.Size {
			continue
		}
		if file.Size > maxSynchronizedFileBytes {
			return fmt.Errorf("remote file %s exceeds synchronization limit of %d bytes", path, maxSynchronizedFileBytes)
		}
		content, err := remote.DownloadFile(ctx, "/workspace/"+path, maxSynchronizedFileBytes)
		if err != nil {
			return err
		}
		if err := writeLocalFile(localRoot, file, content); err != nil {
			return err
		}
	}
	return nil
}

func deleteRemoteFiles(ctx context.Context, remote remoteWorkspace, paths []string) error {
	content, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	script := "import json,os\nfor p in json.loads(" + fmt.Sprintf("%q", string(content)) + "):\n f=os.path.abspath(os.path.join('/workspace',p))\n if os.path.commonpath(['/workspace',f])=='/workspace' and os.path.isfile(f): os.remove(f)\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	result, err := remote.Run(ctx, domainsandbox.CommandRequest{
		Command: "python -c \"import base64;exec(base64.b64decode('" + encoded + "'))\"",
		WorkDir: "/workspace",
	})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("delete stale OpenSandbox files failed: %s", strings.TrimSpace(result.Stderr))
	}
	return nil
}
