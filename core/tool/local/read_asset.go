package local

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"myai/core/adapter/documentparser"
	"myai/core/imageasset"
	"myai/core/port/files"
	toolruntime "myai/core/tool/runtimecontext"
	tooldef "myai/core/tool/tool"
)

const defaultReadAssetMaxBytes = 10 * 1024 * 1024

type ReadAssetTool struct {
	files  files.Service
	lookup files.SessionAssetLookup
	scoped bool
}

type readAssetArgs struct {
	AssetID  string `json:"asset_id"`
	MaxBytes int64  `json:"max_bytes"`
	MaxChars int    `json:"max_chars"`
}

type readAssetResult struct {
	AssetID        string                `json:"asset_id"`
	FileName       string                `json:"file_name,omitempty"`
	ContentType    string                `json:"content_type,omitempty"`
	Size           int64                 `json:"size,omitempty"`
	Truncated      bool                  `json:"truncated"`
	Parsed         documentparser.Result `json:"parsed"`
	ImageReference bool                  `json:"image_reference,omitempty"`
}

// Production always supplies a session lookup. The no-lookup form is kept for
// existing standalone callers with a trusted, already scoped files service.
func NewReadAssetToolWithFiles(service files.Service, lookup ...files.SessionAssetLookup) *ReadAssetTool {
	tool := &ReadAssetTool{files: service}
	if len(lookup) > 0 {
		tool.lookup, tool.scoped = lookup[0], true
	}
	return tool
}

func (t *ReadAssetTool) Name() string {
	return "read_asset"
}

func (t *ReadAssetTool) Description() string {
	return "Read an asset attached to this session by asset_id. Text files are parsed. Images return only a reference, and supported JPEG or PNG pixels may be supplied separately after the tool batch."
}

func (t *ReadAssetTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"asset_id": map[string]any{
				"type":        "string",
				"description": "The asset ID of a file accessible in this session. Do not use a download URL or short code.",
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

	var authorized files.Asset
	if t.scoped {
		execution, ok := toolruntime.CurrentExecution(ctx)
		if !ok || strings.TrimSpace(execution.SessionID) == "" || t.lookup == nil {
			return tooldef.ToolOutput{}, files.ErrSessionAssetUnavailable
		}
		authorized, err = t.lookup(ctx, execution.SessionID, input.AssetID)
		if err != nil || authorized.AssetID != input.AssetID {
			if ctx.Err() != nil {
				return tooldef.ToolOutput{}, ctx.Err()
			}
			return tooldef.ToolOutput{}, files.ErrSessionAssetUnavailable
		}
		if imageasset.LooksLikeImage(authorized.FileName, authorized.ContentType) {
			// The generation boundary resolves and verifies image bytes, sharing that
			// work with the user's attachments in the same turn.
			return readAssetImageReference(authorized, authorized.Size > input.MaxBytes)
		}
	}

	download, err := t.files.Download(ctx, files.DownloadRequest{
		AssetID:  input.AssetID,
		MaxBytes: input.MaxBytes,
	})
	if err != nil {
		if !t.scoped {
			return tooldef.ToolOutput{}, err
		}
		if ctx.Err() != nil {
			return tooldef.ToolOutput{}, ctx.Err()
		}
		return tooldef.ToolOutput{}, errors.New("could not read the attachment; try reattaching it")
	}
	asset := download.Asset
	if t.scoped && (asset.AssetID != input.AssetID || asset.Size != authorized.Size ||
		(authorized.BusinessID != "" && asset.BusinessID != authorized.BusinessID) ||
		(authorized.Checksum != "" && !strings.EqualFold(asset.Checksum, authorized.Checksum))) {
		return tooldef.ToolOutput{}, files.ErrSessionAssetUnavailable
	}
	if asset.AssetID == "" {
		asset.AssetID = input.AssetID
	}
	if imageasset.LooksLikeImage(asset.FileName, asset.ContentType) || strings.HasPrefix(http.DetectContentType(download.Data), "image/") {
		return readAssetImageReference(asset, download.Truncated)
	}

	parsed := documentparser.Parse(ctx, documentparser.Request{
		FileName:    asset.FileName,
		ContentType: asset.ContentType,
		Data:        download.Data,
		MaxChars:    input.MaxChars,
	})
	// Never expose a raw prefix of an unrecognized binary in tool records.
	delete(parsed.Metadata, "head_hex")
	parsed.Truncated = parsed.Truncated || download.Truncated
	if download.Truncated && parsed.Message == "" {
		parsed.Message = "The asset exceeded max_bytes, so only metadata and any downloaded prefix are available."
	}
	return formatReadAssetResult(asset, parsed, false)
}

func readAssetImageReference(asset files.Asset, truncated bool) (tooldef.ToolOutput, error) {
	message := "This result contains only an image reference. If image content is supplied separately, use it for visual details."
	if truncated {
		message = "Image content was not provided: the attachment exceeds max_bytes. Try a smaller JPEG or PNG."
	}
	return formatReadAssetResult(asset, documentparser.Result{
		Kind: "image", Message: message, Truncated: truncated,
	}, !truncated)
}

func formatReadAssetResult(asset files.Asset, parsed documentparser.Result, imageReference bool) (tooldef.ToolOutput, error) {
	result := readAssetResult{
		AssetID: asset.AssetID, FileName: asset.FileName, ContentType: asset.ContentType,
		Size: asset.Size, Truncated: parsed.Truncated, Parsed: parsed, ImageReference: imageReference,
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
	if !files.IsSessionAssetID(input.AssetID) {
		return readAssetArgs{}, errors.New("asset_id is invalid")
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = defaultReadAssetMaxBytes
	}
	input.MaxBytes = min(input.MaxBytes, files.MaxFileBytes)
	if input.MaxChars <= 0 {
		input.MaxChars = documentparser.DefaultMaxTextChars
	}
	return input, nil
}
