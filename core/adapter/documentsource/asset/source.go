package assetsource

import (
	"context"
	"errors"
	"fmt"

	"myai/core/asset"
	domainknowledge "myai/core/domain/knowledge"
	knowledgeport "myai/core/port/knowledge"
)

type Source struct {
	Client *asset.Client
}

var _ knowledgeport.DocumentSourceReader = Source{}

func (source Source) Read(ctx context.Context, request domainknowledge.DocumentSourceRequest) (domainknowledge.DocumentSourceContent, error) {
	if source.Client == nil {
		return domainknowledge.DocumentSourceContent{}, errors.New("asset document source is not configured")
	}
	response, err := source.Client.DownloadAsset(ctx, asset.DownloadAssetRequest{URL: request.URL, Code: request.Code, MaxBytes: request.MaxBytes})
	if err != nil {
		return domainknowledge.DocumentSourceContent{}, err
	}
	if response.Truncated {
		return domainknowledge.DocumentSourceContent{}, fmt.Errorf("document %q exceeds the maximum ingest size", response.FileName)
	}
	content := domainknowledge.DocumentSourceContent{
		FileName: response.FileName, ContentType: response.ContentType,
		Size: int64(len(response.Data)), Data: append([]byte(nil), response.Data...),
	}
	if err := content.Validate(); err != nil {
		return domainknowledge.DocumentSourceContent{}, err
	}
	return content, nil
}
