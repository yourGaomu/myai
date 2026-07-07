package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const defaultUploadTimeout = 60 * time.Second
const defaultDownloadMaxBytes = 20 * 1024 * 1024

type Config struct {
	BaseURL           string
	Timeout           time.Duration
	DefaultTTLSeconds int64
	DefaultMaxVisits  int64
}

type Client struct {
	baseURL           string
	httpClient        *http.Client
	defaultTTLSeconds int64
	defaultMaxVisits  int64
}

type UploadFileRequest struct {
	Reader      io.Reader
	FileName    string
	Title       string
	Scope       string
	ContentType string
	TTLSeconds  int64
	MaxVisits   int64
}

type UploadFileResponse struct {
	Code        string     `json:"code"`
	ShortURL    string     `json:"short_url"`
	Bucket      string     `json:"bucket"`
	ObjectKey   string     `json:"object_key"`
	FileName    string     `json:"file_name"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type LinkInfo struct {
	Code              string     `json:"code"`
	Kind              string     `json:"kind"`
	URL               string     `json:"url"`
	Title             string     `json:"title,omitempty"`
	Scope             string     `json:"scope,omitempty"`
	ObjectFileName    string     `json:"object_file_name,omitempty"`
	ObjectContentType string     `json:"object_content_type,omitempty"`
	ObjectSize        int64      `json:"object_size,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

type DownloadAssetRequest struct {
	URL      string
	Code     string
	MaxBytes int64
}

type DownloadAssetResponse struct {
	URL         string
	Code        string
	FileName    string
	ContentType string
	Size        int64
	Data        []byte
	Truncated   bool
}

func NewClient(config Config) (*Client, error) {
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		return nil, errors.New("asset shortener base url is empty")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("asset shortener base url must use http or https: %s", baseURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("asset shortener base url host is empty: %s", baseURL)
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultUploadTimeout
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		defaultTTLSeconds: config.DefaultTTLSeconds,
		defaultMaxVisits:  config.DefaultMaxVisits,
	}, nil
}

func (c *Client) ShortURL(code string) string {
	return c.endpoint("/s/" + strings.TrimSpace(code))
}

func (c *Client) UploadFile(ctx context.Context, request UploadFileRequest) (UploadFileResponse, error) {
	if c == nil {
		return UploadFileResponse{}, errors.New("asset client is nil")
	}
	if request.Reader == nil {
		return UploadFileResponse{}, errors.New("file reader is nil")
	}
	request.FileName = strings.TrimSpace(request.FileName)
	if request.FileName == "" {
		return UploadFileResponse{}, errors.New("file name is empty")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writeFormField(writer, "title", request.Title); err != nil {
		return UploadFileResponse{}, err
	}
	if err := writeFormField(writer, "scope", request.Scope); err != nil {
		return UploadFileResponse{}, err
	}
	ttlSeconds := request.TTLSeconds
	if ttlSeconds == 0 {
		ttlSeconds = c.defaultTTLSeconds
	}
	if ttlSeconds > 0 {
		if err := writer.WriteField("ttl_seconds", strconv.FormatInt(ttlSeconds, 10)); err != nil {
			return UploadFileResponse{}, err
		}
	}
	maxVisits := request.MaxVisits
	if maxVisits == 0 {
		maxVisits = c.defaultMaxVisits
	}
	if maxVisits > 0 {
		if err := writer.WriteField("max_visits", strconv.FormatInt(maxVisits, 10)); err != nil {
			return UploadFileResponse{}, err
		}
	}

	part, err := createFormFile(writer, "file", request.FileName, request.ContentType)
	if err != nil {
		return UploadFileResponse{}, err
	}
	if _, err := io.Copy(part, request.Reader); err != nil {
		return UploadFileResponse{}, err
	}
	if err := writer.Close(); err != nil {
		return UploadFileResponse{}, err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/api/assets"), &body)
	if err != nil {
		return UploadFileResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return UploadFileResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return UploadFileResponse{}, fmt.Errorf("asset upload failed: %s %s", response.Status, strings.TrimSpace(string(data)))
	}

	var payload UploadFileResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return UploadFileResponse{}, err
	}
	if strings.TrimSpace(payload.ShortURL) == "" {
		return UploadFileResponse{}, errors.New("asset upload response short_url is empty")
	}
	return payload, nil
}

func (c *Client) DownloadAsset(ctx context.Context, request DownloadAssetRequest) (DownloadAssetResponse, error) {
	if c == nil {
		return DownloadAssetResponse{}, errors.New("asset client is nil")
	}
	code, shortURL, err := c.resolveShortURL(request)
	if err != nil {
		return DownloadAssetResponse{}, err
	}
	info, err := c.GetLinkInfo(ctx, code)
	if err != nil {
		return DownloadAssetResponse{}, err
	}
	if info.Kind != "object" {
		return DownloadAssetResponse{}, fmt.Errorf("short link %s is not an uploaded asset", code)
	}

	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultDownloadMaxBytes
	}
	if info.ObjectSize > maxBytes {
		return DownloadAssetResponse{
			URL:         shortURL,
			Code:        code,
			FileName:    info.ObjectFileName,
			ContentType: info.ObjectContentType,
			Size:        info.ObjectSize,
			Truncated:   true,
		}, nil
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, shortURL, nil)
	if err != nil {
		return DownloadAssetResponse{}, err
	}
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return DownloadAssetResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return DownloadAssetResponse{}, fmt.Errorf("asset download failed: %s %s", response.Status, strings.TrimSpace(string(data)))
	}

	limited := io.LimitReader(response.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return DownloadAssetResponse{}, err
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}

	contentType := strings.TrimSpace(info.ObjectContentType)
	if contentType == "" {
		contentType = response.Header.Get("Content-Type")
	}

	return DownloadAssetResponse{
		URL:         shortURL,
		Code:        code,
		FileName:    info.ObjectFileName,
		ContentType: contentType,
		Size:        info.ObjectSize,
		Data:        data,
		Truncated:   truncated,
	}, nil
}

func (c *Client) GetLinkInfo(ctx context.Context, code string) (LinkInfo, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return LinkInfo{}, errors.New("asset code is empty")
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/api/links/"+url.PathEscape(code)), nil)
	if err != nil {
		return LinkInfo{}, err
	}
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return LinkInfo{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return LinkInfo{}, fmt.Errorf("asset info failed: %s %s", response.Status, strings.TrimSpace(string(data)))
	}

	var info LinkInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		return LinkInfo{}, err
	}
	return info, nil
}

func (c *Client) endpoint(endpointPath string) string {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return c.baseURL + endpointPath
	}
	parsed.Path = path.Join(parsed.Path, endpointPath)
	return parsed.String()
}

func (c *Client) resolveShortURL(request DownloadAssetRequest) (string, string, error) {
	code := strings.TrimSpace(request.Code)
	if code != "" {
		return code, c.ShortURL(code), nil
	}

	rawURL := strings.TrimSpace(request.URL)
	if rawURL == "" {
		return "", "", errors.New("asset url or code is required")
	}
	code, err := c.codeFromShortURL(rawURL)
	if err != nil {
		return "", "", err
	}
	return code, rawURL, nil
}

func (c *Client) codeFromShortURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != base.Scheme || parsed.Host != base.Host {
		return "", fmt.Errorf("asset url must use configured short-link host: %s", base.Host)
	}

	basePath := strings.TrimRight(base.EscapedPath(), "/")
	assetPath := parsed.EscapedPath()
	if basePath != "" {
		if assetPath != basePath && !strings.HasPrefix(assetPath, basePath+"/") {
			return "", fmt.Errorf("asset url path is outside configured short-link base path: %s", assetPath)
		}
		assetPath = strings.TrimPrefix(assetPath, basePath)
	}

	parts := strings.Split(strings.Trim(assetPath, "/"), "/")
	if len(parts) != 2 || parts[0] != "s" || parts[1] == "" {
		return "", fmt.Errorf("asset url must look like %s", c.ShortURL("<code>"))
	}
	code, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

func writeFormField(writer *multipart.Writer, key string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return writer.WriteField(key, value)
}

func createFormFile(writer *multipart.Writer, fieldName string, fileName string, contentType string) (io.Writer, error) {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(fieldName), escapeQuotes(fileName)))
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set("Content-Type", contentType)
	return writer.CreatePart(header)
}

func escapeQuotes(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}
