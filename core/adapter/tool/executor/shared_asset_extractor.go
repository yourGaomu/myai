package executor

import (
	"encoding/json"
	"strings"
	"time"

	toolcommand "myai/core/application/tool/command"
	domaintool "myai/core/domain/tool"
)

type SharedAssetExtractor struct{}

func (SharedAssetExtractor) Extract(command toolcommand.AssetExtraction) (domaintool.SharedAsset, bool) {
	// 只有 share_file 的成功 JSON 结果可以转换为 Asset，其他工具输出保持普通文本。
	if command.Call.Name != "share_file" || strings.TrimSpace(command.Result) == "" {
		return domaintool.SharedAsset{}, false
	}

	var payload sharedAssetResultDTO
	if err := json.Unmarshal([]byte(command.Result), &payload); err != nil {
		return domaintool.SharedAsset{}, false
	}
	payload.ShortURL = strings.TrimSpace(payload.ShortURL)
	if payload.ShortURL == "" {
		return domaintool.SharedAsset{}, false
	}

	var expiresAt *time.Time
	if value := strings.TrimSpace(payload.ExpiresAt); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			expiresAt = &parsed
		}
	}

	return domaintool.SharedAsset{
		SessionID:   command.SessionID,
		RequestID:   command.RequestID,
		ToolCallID:  command.Call.ID,
		ToolName:    command.Call.Name,
		LocalPath:   strings.TrimSpace(payload.Path),
		FileName:    strings.TrimSpace(payload.FileName),
		ContentType: strings.TrimSpace(payload.ContentType),
		Size:        payload.Size,
		ShortURL:    payload.ShortURL,
		ShortCode:   strings.TrimSpace(payload.Code),
		ExpiresAt:   expiresAt,
		CreatedAt:   command.CreatedAt,
	}, true
}
