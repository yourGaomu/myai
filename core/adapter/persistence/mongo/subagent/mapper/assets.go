package mapper

import (
	sharedpo "myai/core/adapter/persistence/mongo/po"
	domaintool "myai/core/domain/tool"
)

func taskAssetDocuments(assets []domaintool.SharedAsset) []sharedpo.AssetDocument {
	if assets == nil {
		return nil
	}
	items := make([]sharedpo.AssetDocument, 0, len(assets))
	for _, asset := range assets {
		items = append(items, sharedpo.AssetDocument{
			ID: asset.ID, SessionID: asset.SessionID, RequestID: asset.RequestID,
			ToolCallID: asset.ToolCallID, ToolName: asset.ToolName, LocalPath: asset.LocalPath,
			FileName: asset.FileName, ContentType: asset.ContentType, Size: asset.Size,
			ShortURL: asset.ShortURL, ShortCode: asset.ShortCode,
			ExpiresAt: cloneTime(asset.ExpiresAt), CreatedAt: asset.CreatedAt,
		})
	}
	return items
}

func taskAssetDomains(documents []sharedpo.AssetDocument) []domaintool.SharedAsset {
	if documents == nil {
		return nil
	}
	items := make([]domaintool.SharedAsset, 0, len(documents))
	for _, asset := range documents {
		items = append(items, domaintool.SharedAsset{
			ID: asset.ID, SessionID: asset.SessionID, RequestID: asset.RequestID,
			ToolCallID: asset.ToolCallID, ToolName: asset.ToolName, LocalPath: asset.LocalPath,
			FileName: asset.FileName, ContentType: asset.ContentType, Size: asset.Size,
			ShortURL: asset.ShortURL, ShortCode: asset.ShortCode,
			ExpiresAt: cloneTime(asset.ExpiresAt), CreatedAt: asset.CreatedAt,
		})
	}
	return items
}
