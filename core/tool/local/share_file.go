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

	"myai/core/pathpolicy"
	"myai/core/port/files"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

const maxShareFileBytes = files.MaxFileBytes

type ShareFileTool struct {
	workspace string
	files     files.Service
}

type shareFileArgs struct {
	Path       string `json:"path"`
	Title      string `json:"title"`
	TTLSeconds int64  `json:"ttl_seconds"`
	MaxVisits  int64  `json:"max_visits"`
}

type shareFileResult struct {
	AssetID     string `json:"asset_id"`
	BusinessID  string `json:"business_id,omitempty"`
	Path        string `json:"path"`
	ShortURL    string `json:"short_url"`
	Code        string `json:"code"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	MaxVisits   int64  `json:"max_visits"`
}

func NewShareFileToolWithWorkspaceAndFiles(workspace string, service files.Service) *ShareFileTool {
	return &ShareFileTool{workspace: workspace, files: service}
}

func (t *ShareFileTool) Name() string {
	return "share_file"
}

func (t *ShareFileTool) Description() string {
	return "Share a local workspace file through the configured MCP files service and return its asset ID and short URL for preview or download on another device. Sharing publishes the file and requires explicit user consent."
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
			"ttl_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional lifetime in seconds. Zero or omitted means no expiry.",
			},
			"max_visits": map[string]any{
				"type":        "integer",
				"minimum":     -1,
				"description": "Optional access limit: omitted or -1 means unlimited; zero disables access; a positive value limits visits.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ShareFileTool) Permission() tooldef.Permission {
	return tooldef.PermissionExecute
}

func (t *ShareFileTool) RequiresExplicitApproval() bool {
	return true
}

func (t *ShareFileTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if t.files == nil {
		return tooldef.ToolOutput{}, errors.New("MCP files service is not configured")
	}

	workspace, err := toolWorkspace(toolruntime.WorkspaceRoot(ctx, t.workspace))
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	input, err := normalizeShareFileArgs(workspace, args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if err := ctx.Err(); err != nil {
		return tooldef.ToolOutput{}, err
	}

	info, err := os.Stat(input.Path)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if !info.Mode().IsRegular() {
		return tooldef.ToolOutput{}, errors.New("only regular workspace files can be shared")
	}
	if info.Size() > maxShareFileBytes {
		return tooldef.ToolOutput{}, fmt.Errorf("file is too large to share: %s (%d bytes, max %d bytes)", filepath.ToSlash(relativePath(workspace, input.Path)), info.Size(), maxShareFileBytes)
	}
	if (pathpolicy.Policy{WorkspaceRoot: workspace}).RequiresApproval(input.Path) {
		return tooldef.ToolOutput{}, errors.New("refusing to share sensitive or protected file")
	}

	file, err := os.Open(input.Path)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() > maxShareFileBytes {
		return tooldef.ToolOutput{}, errors.New("workspace file changed before sharing")
	}

	response, err := t.files.Share(ctx, files.ShareRequest{
		UploadRequest: files.UploadRequest{
			Reader:      file,
			FileName:    filepath.Base(input.Path),
			ContentType: contentTypeFromPath(input.Path),
			Size:        opened.Size(),
		},
		Title:      input.Title,
		TTLSeconds: input.TTLSeconds,
		MaxVisits:  input.MaxVisits,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}

	result := shareFileResult{
		AssetID:     response.AssetID,
		BusinessID:  response.BusinessID,
		Path:        filepath.ToSlash(relativePath(workspace, input.Path)),
		ShortURL:    response.ShortURL,
		Code:        response.Code,
		FileName:    response.FileName,
		ContentType: response.ContentType,
		Size:        response.Size,
		MaxVisits:   response.MaxVisits,
	}
	if response.ExpiresAt != nil {
		result.ExpiresAt = response.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	return tooldef.SuccessOutput(string(output)), nil
}

func normalizeShareFileArgs(workspace string, args json.RawMessage) (shareFileArgs, error) {
	input := shareFileArgs{MaxVisits: -1}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return shareFileArgs{}, err
		}
	}

	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return shareFileArgs{}, errors.New("path is empty")
	}
	policy := pathpolicy.Policy{WorkspaceRoot: workspace}
	if policy.RequiresApproval(input.Path) {
		return shareFileArgs{}, errors.New("refusing to share sensitive or protected file")
	}
	path, err := pathpolicy.Resolve(workspace, input.Path)
	if err != nil {
		return shareFileArgs{}, err
	}
	input.Path = path

	input.Title = strings.TrimSpace(input.Title)
	if input.TTLSeconds < 0 {
		input.TTLSeconds = 0
	}
	if input.MaxVisits < -1 {
		return input, errors.New("max_visits must be -1, zero, or a positive integer")
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
	return pathpolicy.IsSensitive(name)
}
