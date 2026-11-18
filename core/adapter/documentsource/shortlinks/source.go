package shortlinkssource

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainknowledge "myai/core/domain/knowledge"
	"myai/core/port/files"
	knowledgeport "myai/core/port/knowledge"
)

type Source struct {
	Client files.Service
}

var _ knowledgeport.DocumentSourceReader = Source{}

func (source Source) Read(ctx context.Context, request domainknowledge.DocumentSourceRequest) (domainknowledge.DocumentSourceContent, error) {
	if source.Client == nil {
		return domainknowledge.DocumentSourceContent{}, errors.New("shortlinks document source is not configured")
	}
	assetID := strings.TrimSpace(request.AssetID)
	if assetID == "" {
		return domainknowledge.DocumentSourceContent{}, errors.New("document source asset_id is required")
	}
	response, err := source.Client.Download(ctx, files.DownloadRequest{AssetID: assetID, MaxBytes: request.MaxBytes})
	if err != nil {
		return domainknowledge.DocumentSourceContent{}, err
	}
	if response.Truncated || request.MaxBytes > 0 && int64(len(response.Data)) > request.MaxBytes {
		return domainknowledge.DocumentSourceContent{}, fmt.Errorf("document %q exceeds the maximum ingest size", response.Asset.FileName)
	}
	content := domainknowledge.DocumentSourceContent{
		FileName: response.Asset.FileName, ContentType: response.Asset.ContentType,
		Size: int64(len(response.Data)), Data: append([]byte(nil), response.Data...),
	}
	if err := content.Validate(); err != nil {
		return domainknowledge.DocumentSourceContent{}, err
	}
	return content, nil
}
