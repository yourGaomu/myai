package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"

	domaintool "myai/core/domain/tool"
	"myai/core/imageasset"
	"myai/core/pathpolicy"
	"myai/core/port/files"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

type AttachImageTool struct {
	workspace      string
	files          files.Service
	localRegistrar files.SessionAssetRegistrar
}

type attachImageResult struct {
	AssetID     string `json:"asset_id"`
	BusinessID  string `json:"business_id,omitempty"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Path        string `json:"path"`
	Checksum    string `json:"checksum,omitempty"`
}

func NewAttachImageToolWithWorkspaceAndFiles(workspace string, service files.Service, registrars ...files.SessionAssetRegistrar) *AttachImageTool {
	tool := &AttachImageTool{workspace: workspace, files: service}
	if len(registrars) > 0 {
		tool.localRegistrar = registrars[0]
	}
	return tool
}

func (*AttachImageTool) Name() string { return "attach_image" }

func (*AttachImageTool) Description() string {
	return "Attach a JPEG, PNG, GIF, or WebP from the current workspace to this chat as a private asset. Each attachment requires explicit user approval."
}

func (*AttachImageTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to a regular JPEG, PNG, GIF, or WebP inside the current workspace, up to 16 MiB.",
			},
		},
		"required": []string{"path"},
	}
}

func (*AttachImageTool) Permission() tooldef.Permission { return tooldef.PermissionExecute }

func (*AttachImageTool) RequiresExplicitApproval() bool { return true }

func (t *AttachImageTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if err := ctx.Err(); err != nil {
		return tooldef.ToolOutput{}, err
	}
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tooldef.ToolOutput{}, fmt.Errorf("decode attach_image arguments: %w", err)
	}
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return tooldef.ToolOutput{}, errors.New("path is required")
	}
	data, relative, err := readWorkspaceAttachment(ctx, t.workspace, path)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	_, contentType, err := imageasset.Decode(data)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if err := ctx.Err(); err != nil {
		return tooldef.ToolOutput{}, err
	}

	fileName, size := filepath.Base(relative), int64(len(data))
	assetID, businessID, checksum := "", "", ""
	if t.files != nil {
		asset, err := t.files.Upload(ctx, files.UploadRequest{
			Reader: bytes.NewReader(data), FileName: fileName, ContentType: contentType, Size: size,
		})
		if err != nil {
			return tooldef.ToolOutput{}, err
		}
		assetID, businessID, checksum = asset.AssetID, asset.BusinessID, asset.Checksum
		if !files.IsSessionAssetID(assetID) {
			return tooldef.ToolOutput{}, errors.New("upload backend returned an invalid asset_id")
		}
	} else {
		assetID = "ast_local_" + uuid.NewString()
		sum := sha256.Sum256(data)
		checksum = hex.EncodeToString(sum[:])
	}
	result := attachImageResult{
		AssetID: assetID, BusinessID: businessID, FileName: fileName, ContentType: contentType,
		Size: size, Path: filepath.ToSlash(relative), Checksum: checksum,
	}
	if t.files == nil && t.localRegistrar != nil {
		execution, _ := toolruntime.CurrentExecution(ctx)
		workspaceRoot, err := toolWorkspace(toolruntime.WorkspaceRoot(ctx, t.workspace))
		if err != nil {
			return tooldef.ToolOutput{}, err
		}
		if err := t.localRegistrar.RegisterLocalSessionAsset(ctx, execution.SessionID, files.Asset{AssetID: assetID, FileName: fileName, ContentType: contentType, Size: size, Checksum: checksum}.WithLocalImage(result.Path, workspaceRoot)); err != nil {
			return tooldef.ToolOutput{}, err
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	output := tooldef.SuccessOutput(string(encoded))
	// The extractor binds this receipt to the actual tool call and persists it
	// with the rest of the tool batch. Upload receipts carry no sharing fields.
	output.SharedAsset = &domaintool.SharedAsset{
		AssetID: result.AssetID, BusinessID: result.BusinessID, FileName: result.FileName,
		ContentType: result.ContentType, Size: result.Size, LocalPath: result.Path, Checksum: result.Checksum,
	}
	return output, nil
}

func readWorkspaceAttachment(ctx context.Context, fallback, path string) ([]byte, string, error) {
	execution, _ := toolruntime.CurrentExecution(ctx)
	scoped := execution.SandboxID != "" || execution.ParentSessionID != ""
	if scoped && strings.TrimSpace(execution.WorkspaceRoot) == "" {
		return nil, "", errors.New("session workspace is unavailable")
	}
	workspace := strings.TrimSpace(toolruntime.WorkspaceRoot(ctx, fallback))
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return nil, "", err
		}
	}
	policy := pathpolicy.Policy{WorkspaceRoot: workspace}
	if policy.RequiresApproval(path) {
		return nil, "", errors.New("refusing to attach a protected or out-of-workspace file")
	}
	resolved, err := policy.Resolve(".")
	if err != nil {
		return nil, "", err
	}

	// Walk the submitted path, since canonicalizing it would hide links. Dot-dot
	// components could otherwise hide a skipped link during lexical cleaning.
	slashPath := strings.ReplaceAll(path, "\\", "/")
	for part := range strings.SplitSeq(slashPath, "/") {
		if part == ".." {
			return nil, "", errors.New("attachment path cannot contain parent traversals")
		}
	}
	relative := filepath.Clean(filepath.FromSlash(slashPath))
	if filepath.IsAbs(relative) {
		relative, err = filepath.Rel(resolved.Abs, relative)
		if err != nil {
			return nil, "", err
		}
	}
	if relative == "." || !filepath.IsLocal(relative) {
		return nil, "", errors.New("attachment path must name a file inside the workspace")
	}

	// A sandbox/child uses its prepared local root, including an OpenSandbox
	// mirror. Reject a replaced root and pin each checked directory by handle.
	cleanWorkspace := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(workspace, "\\", "/")))
	if scoped {
		lexical, err := filepath.Abs(cleanWorkspace)
		if err != nil {
			return nil, "", err
		}
		if relative, err := filepath.Rel(lexical, resolved.Abs); err != nil || relative != "." {
			return nil, "", errors.New("session workspace resolves through a link")
		}
	}
	statRoot := os.Stat
	if scoped {
		statRoot = os.Lstat
	}
	before, err := statRoot(cleanWorkspace)
	if err != nil {
		return nil, "", err
	}
	if !before.IsDir() || before.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil, "", errors.New("attachment workspace is not a regular directory")
	}
	root, err := os.OpenRoot(resolved.Abs)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = root.Close() }()
	if !sameAttachmentDirectory(root, before) {
		return nil, "", errors.New("attachment workspace changed during open")
	}

	parts := strings.Split(relative, string(filepath.Separator))
	for _, name := range parts[:len(parts)-1] {
		before, err := root.Lstat(name)
		if err != nil {
			return nil, "", err
		}
		if !before.IsDir() || before.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return nil, "", errors.New("attachment path contains a link or non-directory component")
		}
		next, err := root.OpenRoot(name)
		if err != nil {
			return nil, "", err
		}
		if !sameAttachmentDirectory(next, before) {
			_ = next.Close()
			return nil, "", errors.New("attachment directory changed during open")
		}
		_ = root.Close()
		root = next
	}

	name := parts[len(parts)-1]
	info, err := root.Lstat(name)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("attach_image requires a regular file without symlinks")
	}
	if info.Size() > imageasset.MaxSourceBytes {
		return nil, "", imageasset.ErrTooLarge
	}
	// Avoid blocking on Unix if a regular file is replaced with a FIFO before
	// opening. Windows ignores O_NONBLOCK for this rooted file open.
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, "", errors.New("attachment file changed during open")
	}
	if opened.Size() > imageasset.MaxSourceBytes {
		return nil, "", imageasset.ErrTooLarge
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	data, err := io.ReadAll(io.LimitReader(file, imageasset.MaxSourceBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > imageasset.MaxSourceBytes {
		return nil, "", imageasset.ErrTooLarge
	}
	return data, relative, ctx.Err()
}

func sameAttachmentDirectory(root *os.Root, before os.FileInfo) bool {
	opened, err := root.Stat(".")
	return err == nil && os.SameFile(before, opened)
}
