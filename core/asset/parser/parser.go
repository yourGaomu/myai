package parser

import (
	"context"
	"encoding/hex"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const DefaultMaxTextChars = 12000

type Request struct {
	FileName    string
	ContentType string
	Data        []byte
	MaxChars    int
}

type Result struct {
	FileName    string            `json:"file_name,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Size        int               `json:"size"`
	Text        string            `json:"text,omitempty"`
	Truncated   bool              `json:"truncated"`
	Supported   bool              `json:"supported"`
	Kind        string            `json:"kind"`
	Message     string            `json:"message,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func Parse(ctx context.Context, request Request) Result {
	maxChars := request.MaxChars
	if maxChars <= 0 {
		maxChars = DefaultMaxTextChars
	}

	contentType := normalizeContentType(request.ContentType)
	extension := strings.ToLower(filepath.Ext(request.FileName))
	result := Result{
		FileName:    request.FileName,
		ContentType: contentType,
		Size:        len(request.Data),
		Kind:        parserKind(request.FileName, contentType),
		Metadata: map[string]string{
			"extension": extension,
		},
	}

	if isTextAsset(request.FileName, contentType, request.Data) {
		text := string(request.Data)
		truncatedText, truncated := truncateRunes(text, maxChars)
		result.Text = truncatedText
		result.Truncated = truncated
		result.Supported = true
		result.Kind = "text"
		return result
	}

	if isProbablyPDF(request.FileName, contentType) {
		result.Kind = "pdf"
		return parseWithPythonFile(ctx, "pdf", ".pdf", request, result)
	}
	if isProbablyDocx(request.FileName, contentType) {
		result.Kind = "docx"
		result.Message = "DOCX parsing is not enabled yet. Add the Python parser backend to extract document text."
		return result
	}
	if isProbablyXlsx(request.FileName, contentType) {
		result.Kind = "xlsx"
		result.Message = "XLSX parsing is not enabled yet. Add the Python parser backend or excelize parser to extract sheets."
		return result
	}
	if len(request.Data) > 0 {
		result.Metadata["head_hex"] = hex.EncodeToString(request.Data[:min(len(request.Data), 32)])
	}
	result.Message = "This file type is not text-readable yet."
	return result
}

func parseWithPythonFile(ctx context.Context, fileType string, extension string, request Request, fallback Result) Result {
	tempFile, err := os.CreateTemp("", "myai-asset-*"+extension)
	if err != nil {
		fallback.Message = err.Error()
		return fallback
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(request.Data); err != nil {
		fallback.Message = err.Error()
		_ = tempFile.Close()
		return fallback
	}
	if err := tempFile.Close(); err != nil {
		fallback.Message = err.Error()
		return fallback
	}

	parsed, err := parsePython(ctx, fileType, tempPath, request)
	if err != nil {
		fallback.Message = err.Error()
		return fallback
	}
	return parsed
}

func normalizeContentType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return "application/octet-stream"
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		return contentType
	}
	return strings.ToLower(mediaType)
}

func parserKind(fileName string, contentType string) string {
	extension := strings.ToLower(filepath.Ext(fileName))
	switch {
	case isProbablyPDF(fileName, contentType):
		return "pdf"
	case isProbablyDocx(fileName, contentType):
		return "docx"
	case isProbablyXlsx(fileName, contentType):
		return "xlsx"
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case extension != "":
		return strings.TrimPrefix(extension, ".")
	default:
		return "binary"
	}
}

func isTextAsset(fileName string, contentType string, data []byte) bool {
	if strings.HasPrefix(contentType, "text/") {
		return validText(data)
	}
	switch contentType {
	case "application/json", "application/xml", "application/x-yaml", "application/yaml":
		return validText(data)
	}
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".txt", ".md", ".markdown", ".go", ".json", ".yaml", ".yml", ".toml", ".xml", ".html", ".css", ".js", ".ts", ".tsx", ".jsx", ".py", ".java", ".kt", ".rs", ".c", ".cpp", ".h", ".hpp", ".sql", ".log", ".csv":
		return validText(data)
	default:
		return false
	}
}

func validText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func isProbablyPDF(fileName string, contentType string) bool {
	return contentType == "application/pdf" || strings.EqualFold(filepath.Ext(fileName), ".pdf")
}

func isProbablyDocx(fileName string, contentType string) bool {
	return contentType == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || strings.EqualFold(filepath.Ext(fileName), ".docx")
}

func isProbablyXlsx(fileName string, contentType string) bool {
	return contentType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || strings.EqualFold(filepath.Ext(fileName), ".xlsx")
}

func truncateRunes(text string, maxChars int) (string, bool) {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text, false
	}
	return string(runes[:maxChars]), true
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
