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
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const defaultUploadTimeout = 60 * time.Second
const defaultDownloadMaxBytes = 20 * 1024 * 1024

type Config struct {
	BaseURL           string
	APIKey            string
	Timeout           time.Duration
	DefaultTTLSeconds int64
	DefaultMaxVisits  int64
}

type Client struct {
	baseURL           string
	apiKey            string
	httpClient        *http.Client
	managementClient  *http.Client
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
		return nil, errors.New("asset shortener base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("asset shortener base url must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("asset shortener base url host is empty")
	}
	if parsed.User != nil {
		return nil, errors.New("asset shortener base url must not contain credentials; configure asset.api_key instead")
	}
	apiKey := strings.TrimSpace(config.APIKey)
	if strings.ContainsAny(apiKey, "\r\n") {
		return nil, errors.New("asset api key must not contain line breaks")
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultUploadTimeout
	}

	client := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		defaultTTLSeconds: config.DefaultTTLSeconds,
		defaultMaxVisits:  config.DefaultMaxVisits,
	}
	// Keep management credentials separate from public short-link downloads.
	client.managementClient = &http.Client{
		Timeout:       timeout,
		CheckRedirect: client.checkManagementRedirect,
	}
	return client, nil
}

func (c *Client) checkManagementRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("asset management request stopped after 10 redirects")
	}
	initial := via[0].URL
	if request.URL.Scheme != initial.Scheme || !strings.EqualFold(request.URL.Host, initial.Host) || request.URL.User != nil {
		return errors.New("asset management request cannot redirect outside the configured origin")
	}
	// Never forward a management key to a public short link or object path,
	// including when it is served by the same origin.
	managementBase, _ := url.Parse(c.endpoint("/api/"))
	managementPath := strings.TrimRight(managementBase.Path, "/") + "/"
	if !strings.HasPrefix(path.Clean(request.URL.Path), managementPath) {
		return errors.New("asset management request cannot redirect outside the management API")
	}
	return nil
}

func (c *Client) requireAPIKey() error {
	if c.apiKey == "" {
		return errors.New("asset api key is required; configure asset.api_key or ASSET_API_KEY")
	}
	return nil
}

func (c *Client) ShortURL(code string) string {
	return c.endpoint("/s/" + strings.TrimSpace(code))
}

func (c *Client) UploadFile(ctx context.Context, request UploadFileRequest) (UploadFileResponse, error) {
	if c == nil {
		return UploadFileResponse{}, errors.New("asset client is nil")
	}
	if err := c.requireAPIKey(); err != nil {
		return UploadFileResponse{}, err
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
		return UploadFileResponse{}, c.managementError(err)
	}
	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)

	response, err := c.managementClient.Do(httpRequest)
	if err != nil {
		return UploadFileResponse{}, c.managementError(err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return UploadFileResponse{}, c.managementResponseError("upload", response)
	}

	var payload UploadFileResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return UploadFileResponse{}, safeRequestError("asset upload response could not be read", err)
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
		return DownloadAssetResponse{}, errors.New("short link is not an uploaded asset")
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
		return DownloadAssetResponse{}, safeRequestError("asset download request failed", err)
	}
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return DownloadAssetResponse{}, safeRequestError("asset download request failed", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return DownloadAssetResponse{}, fmt.Errorf("asset download failed: HTTP %d %s", response.StatusCode, http.StatusText(response.StatusCode))
	}

	limited := io.LimitReader(response.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return DownloadAssetResponse{}, safeRequestError("asset download response could not be read", err)
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
	if c == nil {
		return LinkInfo{}, errors.New("asset client is nil")
	}
	if err := c.requireAPIKey(); err != nil {
		return LinkInfo{}, err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return LinkInfo{}, errors.New("asset code is empty")
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/api/links/"+url.PathEscape(code)), nil)
	if err != nil {
		return LinkInfo{}, c.managementError(err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	response, err := c.managementClient.Do(httpRequest)
	if err != nil {
		return LinkInfo{}, c.managementError(err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return LinkInfo{}, c.managementResponseError("info", response)
	}

	var info LinkInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		return LinkInfo{}, safeRequestError("asset info response could not be read", err)
	}
	return info, nil
}

func (c *Client) managementError(err error) error {
	return safeRequestError("asset management request failed", err)
}

func safeRequestError(message string, err error) error {
	// Never retain a transport or parser error: even an inner url.Error.Err can
	// contain a full Location header, Authorization value or signed object URL.
	// Rebuild only safe cancellation/timeout categories from trusted sentinels.
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s: %w", message, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s: %w", message, context.DeadlineExceeded)
	case errors.Is(err, os.ErrDeadlineExceeded):
		return fmt.Errorf("%s: %w", message, os.ErrDeadlineExceeded)
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return fmt.Errorf("%s: %w", message, os.ErrDeadlineExceeded)
	}
	return errors.New(message)
}

func (c *Client) managementResponseError(operation string, response *http.Response) error {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("asset %s authentication failed (%d); check asset.api_key or ASSET_API_KEY", operation, response.StatusCode)
	default:
		// A proxy or server may echo Authorization in its error response. Do not
		// forward response bodies into tool results or conversation history.
		return fmt.Errorf("asset %s failed: HTTP %d %s", operation, response.StatusCode, http.StatusText(response.StatusCode))
	}
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
		return "", errors.New("asset url is invalid")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", errors.New("asset shortener base url is invalid")
	}
	if parsed.User != nil {
		return "", errors.New("asset url must not contain credentials")
	}
	if parsed.Scheme != base.Scheme || parsed.Host != base.Host {
		return "", errors.New("asset url must use configured short-link host")
	}

	basePath := strings.TrimRight(base.EscapedPath(), "/")
	assetPath := parsed.EscapedPath()
	if basePath != "" {
		if assetPath != basePath && !strings.HasPrefix(assetPath, basePath+"/") {
			return "", errors.New("asset url path is outside configured short-link base path")
		}
		assetPath = strings.TrimPrefix(assetPath, basePath)
	}

	parts := strings.Split(strings.Trim(assetPath, "/"), "/")
	if len(parts) != 2 || parts[0] != "s" || parts[1] == "" {
		return "", errors.New("asset url must use the configured short-link base path followed by /s/<code>")
	}
	code, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", errors.New("asset code is invalid")
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
