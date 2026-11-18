package tool

import "time"

type SharedAsset struct {
	ID          string
	AssetID     string
	BusinessID  string
	Checksum    string
	MaxVisits   int64
	SessionID   string
	RequestID   string
	ToolCallID  string
	ToolName    string
	LocalPath   string
	FileName    string
	ContentType string
	Size        int64
	ShortURL    string
	ShortCode   string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
}

func CloneSharedAssets(assets []SharedAsset) []SharedAsset {
	if assets == nil {
		return nil
	}
	cloned := append([]SharedAsset{}, assets...)
	for index := range cloned {
		if cloned[index].ExpiresAt != nil {
			value := *cloned[index].ExpiresAt
			cloned[index].ExpiresAt = &value
		}
	}
	return cloned
}
