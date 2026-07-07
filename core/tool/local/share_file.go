package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"myai/core/asset"
	tooldef "myai/core/tool/tool"
)

const maxShareFileBytes = 50 * 1024 * 1024

type assetUploader interface {
	UploadFile(ctx context.Context, request asset.UploadFileRequest) (asset.UploadFileResponse, error)
}

type ShareFileTool struct {
	workspace string
	uploader  assetUploader
}

type shareFileArgs struct {
	Path       string `json:"path"`
	Title      string `json:"title"`
	Scope      string `json:"scope"`
	TTLSeconds int64  `json:"ttl_seconds"`
	MaxVisits  int64  `json:"max_visits"`
}

type shareFileResult struct {
	Path        string `json:"path"`
	ShortURL    string `json:"short_url"`
	Code        string `json:"code"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

func NewShareFileToolWithWorkspace(workspace string) *ShareFileTool {
	return &ShareFileTool{workspace: workspace}
}

func NewShareFileToolWithWorkspaceAndUploader(workspace string, uploader assetUploader) *ShareFileTool {
	return &ShareFileTool{workspace: workspace, uploader: uploader}
}

func (t *ShareFileTool) Name() string {
	return "share_file"
}

func (t *ShareFileTool) Description() string {
	return "Upload a local workspace file to the configured asset short-link service and return a short URL for mobile preview or download. Use this after creating files the user should open on another device."
}

func (t *ShareFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The workspace file path to upload.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Optional display title for the uploaded asset.",
			},
			"scope": map[string]any{
				"type":        "string",
				"description": "Optional scope tag, such as session id or project name.",
			},
			"ttl_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional expiry time in seconds. Uses configured default when omitted.",
			},
			"max_visits": map[string]any{
				"type":        "integer",
				"description": "Optional maximum visits. Uses configured default when omitted.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ShareFileTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (t *ShareFileTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if t.uploader == nil {
		return "", errors.New("asset short-link service is not configured; set asset.shortener_base_url")
	}

	workspace, err := toolWorkspace(t.workspace)
	if err != nil {
		return "", err
	}
	input, err := normalizeShareFileArgs(workspace, args)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	info, err := os.Stat(input.Path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", input.Path)
	}
	if info.Size() > maxShareFileBytes {
		return "", fmt.Errorf("file is too large to share: %s (%d bytes, max %d bytes)", filepath.ToSlash(relativePath(workspace, input.Path)), info.Size(), maxShareFileBytes)
	}
	if isSensitiveFileName(filepath.Base(input.Path)) {
		return "", fmt.Errorf("refusing to share sensitive file: %s", filepath.ToSlash(relativePath(workspace, input.Path)))
	}

	file, err := os.Open(input.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	response, err := t.uploader.UploadFile(ctx, asset.UploadFileRequest{
		Reader:      file,
		FileName:    filepath.Base(input.Path),
		Title:       input.Title,
		Scope:       input.Scope,
		ContentType: contentTypeFromPath(input.Path),
		TTLSeconds:  input.TTLSeconds,
		MaxVisits:   input.MaxVisits,
	})
	if err != nil {
		return "", err
	}

	result := shareFileResult{
		Path:        filepath.ToSlash(relativePath(workspace, input.Path)),
		ShortURL:    response.ShortURL,
		Code:        response.Code,
		FileName:    response.FileName,
		ContentType: response.ContentType,
		Size:        response.Size,
	}
	if response.ExpiresAt != nil {
		result.ExpiresAt = response.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func normalizeShareFileArgs(workspace string, args json.RawMessage) (shareFileArgs, error) {
	var input shareFileArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return shareFileArgs{}, err
		}
	}

	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return shareFileArgs{}, errors.New("path is empty")
	}
	path, err := cleanWorkspacePath(workspace, input.Path)
	if err != nil {
		return shareFileArgs{}, err
	}
	input.Path = path

	input.Title = strings.TrimSpace(input.Title)
	input.Scope = strings.TrimSpace(input.Scope)
	if input.TTLSeconds < 0 {
		input.TTLSeconds = 0
	}
	if input.MaxVisits < 0 {
		input.MaxVisits = 0
	}
	return input, nil
}

func contentTypeFromPath(path string) string {
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if strings.TrimSpace(contentType) == "" {
		return "application/octet-stream"
	}
	return contentType
}

func isSensitiveFileName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ".env", ".env.local", ".env.production", "id_rsa", "id_ed25519":
		return true
	default:
		return false
	}
}
