package executor

type sharedAssetResultDTO struct {
	Path        string `json:"path"`
	ShortURL    string `json:"short_url"`
	Code        string `json:"code"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ExpiresAt   string `json:"expires_at"`
}
