package local

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"myai/core/asset"
	"myai/core/asset/parser"
	tooldef "myai/core/tool/tool"
)

const defaultReadAssetMaxBytes = 10 * 1024 * 1024

type assetDownloader interface {
	DownloadAsset(ctx context.Context, request asset.DownloadAssetRequest) (asset.DownloadAssetResponse, error)
}

type ReadAssetTool struct {
	downloader assetDownloader
}

type readAssetArgs struct {
	URL      string `json:"url"`
	Code     string `json:"code"`
	MaxBytes int64  `json:"max_bytes"`
	MaxChars int    `json:"max_chars"`
}

type readAssetResult struct {
	URL         string        `json:"url"`
	Code        string        `json:"code"`
	FileName    string        `json:"file_name,omitempty"`
	ContentType string        `json:"content_type,omitempty"`
	Size        int64         `json:"size,omitempty"`
	Truncated   bool          `json:"truncated"`
	Parsed      parser.Result `json:"parsed"`
}

func NewReadAssetToolWithDownloader(downloader assetDownloader) *ReadAssetTool {
	return &ReadAssetTool{downloader: downloader}
}

func (t *ReadAssetTool) Name() string {
	return "read_asset"
}

func (t *ReadAssetTool) Description() string {
	return "Download and parse a file previously uploaded to the configured asset short-link service. Use this when the user uploads a mobile file or provides an uploaded_file short_url."
}

func (t *ReadAssetTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The asset short URL, usually from an uploaded_file short_url field.",
			},
			"code": map[string]any{
				"type":        "string",
				"description": "Optional short-link code. Use this instead of url when only the code is known.",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Maximum bytes to download. Defaults to 10MB and is capped by the configured asset client.",
			},
			"max_chars": map[string]any{
				"type":        "integer",
				"description": "Maximum text characters to return after parsing. Defaults to 12000.",
			},
		},
	}
}

func (t *ReadAssetTool) Permission() tooldef.Permission {
	return tooldef.PermissionRead
}

func (t *ReadAssetTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if t.downloader == nil {
		return "", errors.New("asset short-link service is not configured; set asset.shortener_base_url")
	}
	input, err := normalizeReadAssetArgs(args)
	if err != nil {
		return "", err
	}

	download, err := t.downloader.DownloadAsset(ctx, asset.DownloadAssetRequest{
		URL:      input.URL,
		Code:     input.Code,
		MaxBytes: input.MaxBytes,
	})
	if err != nil {
		return "", err
	}

	parsed := parser.Parse(ctx, parser.Request{
		FileName:    download.FileName,
		ContentType: download.ContentType,
		Data:        download.Data,
		MaxChars:    input.MaxChars,
	})
	parsed.Truncated = parsed.Truncated || download.Truncated
	if download.Truncated && parsed.Message == "" {
		parsed.Message = "The asset exceeded max_bytes, so only metadata and any downloaded prefix are available."
	}

	result := readAssetResult{
		URL:         download.URL,
		Code:        download.Code,
		FileName:    download.FileName,
		ContentType: download.ContentType,
		Size:        download.Size,
		Truncated:   download.Truncated || parsed.Truncated,
		Parsed:      parsed,
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func normalizeReadAssetArgs(args json.RawMessage) (readAssetArgs, error) {
	input := readAssetArgs{
		MaxBytes: defaultReadAssetMaxBytes,
		MaxChars: parser.DefaultMaxTextChars,
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return readAssetArgs{}, err
		}
	}
	input.URL = strings.TrimSpace(input.URL)
	input.Code = strings.TrimSpace(input.Code)
	if input.URL == "" && input.Code == "" {
		return readAssetArgs{}, errors.New("url or code is required")
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = defaultReadAssetMaxBytes
	}
	if input.MaxChars <= 0 {
		input.MaxChars = parser.DefaultMaxTextChars
	}
	return input, nil
}
