package executor

type sharedAssetResultDTO struct {
	AssetID     string `json:"asset_id"`
	BusinessID  string `json:"business_id"`
	Path        string `json:"path"`
	ShortURL    string `json:"short_url"`
	Code        string `json:"code"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ExpiresAt   string `json:"expires_at"`
	MaxVisits   int64  `json:"max_visits"`
}
