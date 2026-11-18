package executor

import (
	"encoding/json"
	"strings"
	"time"

	uuidadapter "myai/core/adapter/id/uuid"
	toolcommand "myai/core/application/tool/command"
	domaintool "myai/core/domain/tool"
)

type SharedAssetExtractor struct{}

func (SharedAssetExtractor) Extract(command toolcommand.AssetExtraction) (domaintool.SharedAsset, bool) {
	if command.Output.Normalized().Status != domaintool.ResultStatusSuccess {
		return domaintool.SharedAsset{}, false
	}

	var asset domaintool.SharedAsset
	if command.Output.SharedAsset != nil {
		// Trusted adapters attach typed metadata before formatting MCP output.
		asset = domaintool.CloneSharedAssets([]domaintool.SharedAsset{*command.Output.SharedAsset})[0]
	} else {
		// Only the local publication tool's canonical JSON is an asset result.
		if command.Call.Name != "share_file" {
			return domaintool.SharedAsset{}, false
		}
		var payload sharedAssetResultDTO
		if err := json.Unmarshal([]byte(command.Output.Content), &payload); err != nil {
			return domaintool.SharedAsset{}, false
		}
		if strings.TrimSpace(payload.ShortURL) == "" {
			return domaintool.SharedAsset{}, false
		}
		asset = domaintool.SharedAsset{
			AssetID: payload.AssetID, BusinessID: payload.BusinessID,
			LocalPath: payload.Path, FileName: payload.FileName, ContentType: payload.ContentType,
			Size: payload.Size, ShortURL: payload.ShortURL, ShortCode: payload.Code, MaxVisits: payload.MaxVisits,
		}
		if value := strings.TrimSpace(payload.ExpiresAt); value != "" {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				asset.ExpiresAt = &parsed
			}
		}
	}

	asset.AssetID = strings.TrimSpace(asset.AssetID)
	if asset.AssetID == "" || asset.Size < 0 || asset.MaxVisits < -1 {
		return domaintool.SharedAsset{}, false
	}
	asset.BusinessID = strings.TrimSpace(asset.BusinessID)
	asset.Checksum = strings.TrimSpace(asset.Checksum)
	asset.LocalPath = strings.TrimSpace(asset.LocalPath)
	asset.FileName = strings.TrimSpace(asset.FileName)
	asset.ContentType = strings.TrimSpace(asset.ContentType)
	asset.ShortURL = strings.TrimSpace(asset.ShortURL)
	asset.ShortCode = strings.TrimSpace(asset.ShortCode)
	// Use one local event ID for streaming and persistence, and bind it to this call.
	asset.ID = (uuidadapter.Generator{}).NewID()
	asset.SessionID = command.SessionID
	asset.RequestID = command.RequestID
	asset.ToolCallID = command.Call.ID
	asset.ToolName = command.Call.Name
	asset.CreatedAt = command.CreatedAt
	return asset, true
}
