package imageasset

import (
	"path"
	"strings"
)

// LooksLikeImage selects candidates from metadata. Only Decode verifies content.
func LooksLikeImage(fileName, contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "image/") {
		return true
	}
	switch strings.ToLower(path.Ext(fileName)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".svg", ".tif", ".tiff", ".heic", ".heif", ".avif", ".ico":
		return true
	}
	return false
}
