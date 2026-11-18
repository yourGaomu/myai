package mcp

import (
	"encoding/hex"
	"encoding/json"
	"mime"
	"net/url"
	"strings"
	"time"
	"unicode"

	domaintool "myai/core/domain/tool"
	filesport "myai/core/port/files"
	tooldef "myai/core/tool/tool"
)

type fileAssetResult struct {
	AssetID        string     `json:"assetId"`
	BusinessID     string     `json:"businessId"`
	FileName       string     `json:"fileName"`
	ContentType    string     `json:"contentType"`
	SizeBytes      *int64     `json:"sizeBytes"`
	ChecksumSHA256 string     `json:"checksumSha256"`
	DeletedAt      *time.Time `json:"deletedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
}

func fileResultID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func normalizedFileOutput(result CallResult, originalName string, args json.RawMessage) tooldef.ToolOutput {
	failed := func() tooldef.ToolOutput {
		return tooldef.FailedOutput(domaintool.ResultStatusFailed, "invalid_mcp_file_result", "short_links returned an invalid structured file result")
	}
	if result.IsError || !json.Valid(result.StructuredContent) {
		return failed()
	}
	var envelope struct {
		Asset     fileAssetResult `json:"asset"`
		ShortURL  string          `json:"shortUrl"`
		Code      string          `json:"code"`
		ExpiresAt *time.Time      `json:"expiresAt"`
		MaxVisits *int64          `json:"maxVisits"`
	}
	if originalName == "get_asset_info" {
		if json.Unmarshal(result.StructuredContent, &envelope.Asset) != nil {
			return failed()
		}
	} else if json.Unmarshal(result.StructuredContent, &envelope) != nil {
		return failed()
	}
	asset := envelope.Asset
	var request struct {
		AssetID  string `json:"assetId"`
		FileName string `json:"fileName"`
	}
	if json.Unmarshal(args, &request) != nil ||
		!fileResultID(asset.AssetID) || !fileResultID(asset.BusinessID) || asset.SizeBytes == nil ||
		*asset.SizeBytes <= 0 || *asset.SizeBytes > filesport.MaxFileBytes || asset.CreatedAt.IsZero() || asset.DeletedAt != nil ||
		asset.FileName == "" || len(asset.FileName) > 512 || strings.ContainsAny(asset.FileName, "/\\") || strings.ContainsFunc(asset.FileName, unicode.IsControl) {
		return failed()
	}
	if originalName != "upload_file_and_share" && (request.AssetID == "" || request.AssetID != asset.AssetID) ||
		originalName == "upload_file_and_share" && request.FileName != asset.FileName {
		return failed()
	}
	if _, _, err := mime.ParseMediaType(asset.ContentType); err != nil {
		return failed()
	}
	if asset.ChecksumSHA256 != "" {
		checksum, err := hex.DecodeString(asset.ChecksumSHA256)
		if err != nil || len(checksum) != 32 {
			return failed()
		}
	}
	var maxVisits int64
	if originalName != "get_asset_info" {
		u, err := url.Parse(envelope.ShortURL)
		if err != nil || len(envelope.ShortURL) > 4096 || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
			u.User != nil || u.RawQuery != "" || u.Fragment != "" || !fileResultID(envelope.Code) ||
			envelope.MaxVisits == nil || *envelope.MaxVisits < -1 || envelope.ExpiresAt != nil && !envelope.ExpiresAt.After(time.Now()) {
			return failed()
		}
		maxVisits = *envelope.MaxVisits
	}
	shared := &domaintool.SharedAsset{AssetID: asset.AssetID, BusinessID: asset.BusinessID, FileName: asset.FileName,
		ContentType: asset.ContentType, Size: *asset.SizeBytes, Checksum: asset.ChecksumSHA256, CreatedAt: asset.CreatedAt,
		ShortURL: envelope.ShortURL, ShortCode: envelope.Code, ExpiresAt: envelope.ExpiresAt, MaxVisits: maxVisits}
	canonical := map[string]any{"asset_id": shared.AssetID, "business_id": shared.BusinessID,
		"file_name": shared.FileName, "content_type": shared.ContentType, "size": shared.Size,
		"checksum": shared.Checksum, "created_at": shared.CreatedAt}
	if shared.ShortURL != "" {
		canonical["short_url"], canonical["code"] = shared.ShortURL, shared.ShortCode
		canonical["expires_at"], canonical["max_visits"] = shared.ExpiresAt, shared.MaxVisits
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return failed()
	}
	output := tooldef.SuccessOutput(string(encoded))
	output.SharedAsset = shared
	return output
}
