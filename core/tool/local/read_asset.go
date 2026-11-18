package local

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"myai/core/adapter/documentparser"
	"myai/core/port/files"
	tooldef "myai/core/tool/tool"
)

const defaultReadAssetMaxBytes = 10 * 1024 * 1024

type ReadAssetTool struct {
	files files.Service
}

type readAssetArgs struct {
	AssetID  string `json:"asset_id"`
	MaxBytes int64  `json:"max_bytes"`
	MaxChars int    `json:"max_chars"`
}

type readAssetResult struct {
	AssetID     string                `json:"asset_id"`
	FileName    string                `json:"file_name,omitempty"`
	ContentType string                `json:"content_type,omitempty"`
	Size        int64                 `json:"size,omitempty"`
	Truncated   bool                  `json:"truncated"`
	Parsed      documentparser.Result `json:"parsed"`
}

func NewReadAssetToolWithFiles(service files.Service) *ReadAssetTool {
	return &ReadAssetTool{files: service}
}

func (t *ReadAssetTool) Name() string {
	return "read_asset"
}

func (t *ReadAssetTool) Description() string {
	return "Download and parse an uploaded file through the configured MCP files service. Use the asset_id returned by file upload or share_file."
}

func (t *ReadAssetTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"asset_id": map[string]any{
				"type":        "string",
				"description": "The asset ID returned by file upload or share_file.",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Maximum bytes to download. Defaults to 10MB and is capped by the files service.",
			},
			"max_chars": map[string]any{
				"type":        "integer",
				"description": "Maximum text characters to return after parsing. Defaults to 12000.",
			},
		},
		"required": []string{"asset_id"},
	}
}

func (t *ReadAssetTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (t *ReadAssetTool) Call(ctx context.Context, args json.RawMessage) (tooldef.ToolOutput, error) {
	if t.files == nil {
		return tooldef.ToolOutput{}, errors.New("MCP files service is not configured")
	}
	input, err := normalizeReadAssetArgs(args)
	if err != nil {
		return tooldef.ToolOutput{}, err
	}

	download, err := t.files.Download(ctx, files.DownloadRequest{
		AssetID:  input.AssetID,
		MaxBytes: input.MaxBytes,
	})
	if err != nil {
		return tooldef.ToolOutput{}, err
	}

	parsed := documentparser.Parse(ctx, documentparser.Request{
		FileName:    download.Asset.FileName,
		ContentType: download.Asset.ContentType,
		Data:        download.Data,
		MaxChars:    input.MaxChars,
	})
	parsed.Truncated = parsed.Truncated || download.Truncated
	if download.Truncated && parsed.Message == "" {
		parsed.Message = "The asset exceeded max_bytes, so only metadata and any downloaded prefix are available."
	}

	result := readAssetResult{
		AssetID:     download.Asset.AssetID,
		FileName:    download.Asset.FileName,
		ContentType: download.Asset.ContentType,
		Size:        download.Asset.Size,
		Truncated:   download.Truncated || parsed.Truncated,
		Parsed:      parsed,
	}
	if result.AssetID == "" {
		result.AssetID = input.AssetID
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return tooldef.ToolOutput{}, err
	}
	resultOutput := tooldef.SuccessOutput(string(output))
	resultOutput.Truncated = result.Truncated
	return resultOutput, nil
}

func normalizeReadAssetArgs(args json.RawMessage) (readAssetArgs, error) {
	input := readAssetArgs{
		MaxBytes: defaultReadAssetMaxBytes,
		MaxChars: documentparser.DefaultMaxTextChars,
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return readAssetArgs{}, err
		}
	}
	input.AssetID = strings.TrimSpace(input.AssetID)
	if input.AssetID == "" {
		return readAssetArgs{}, errors.New("asset_id is required")
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = defaultReadAssetMaxBytes
	}
	if input.MaxChars <= 0 {
		input.MaxChars = documentparser.DefaultMaxTextChars
	}
	return input, nil
}
