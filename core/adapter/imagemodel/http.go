package imagemodel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	googleauth "cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"

	domain "myai/core/domain/imagemodel"
	"myai/core/imageasset"
	port "myai/core/port/imagemodel"
)

const (
	maxResponseBytes     = imageasset.MaxSourceBytes
	maxJSONResponseBytes = 24 * 1024 * 1024
	maxReferenceBytes    = 32 * 1024 * 1024
	maxPromptBytes       = 64 * 1024
)

type GoogleTokenSource interface {
	AccessToken(context.Context) (string, error)
}
type defaultGoogleTokenSource struct {
	mu          sync.Mutex
	credentials *googleauth.Credentials
}

func (s *defaultGoogleTokenSource) AccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	current := s.credentials
	if current == nil {
		found, err := credentials.DetectDefault(&credentials.DetectOptions{
			Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
			Client: &http.Client{Timeout: 15 * time.Second},
		})
		if err != nil || found == nil {
			s.mu.Unlock()
			return "", errors.New("Google Application Default Credentials are unavailable")
		}
		s.credentials, current = found, found
	}
	s.mu.Unlock()
	token, err := current.Token(ctx)
	if err != nil || token == nil || strings.TrimSpace(token.Value) == "" {
		return "", errors.New("Google Application Default Credentials token is unavailable")
	}
	return token.Value, nil
}

type HTTPFactory struct {
	Client       *http.Client
	URLClient    *http.Client
	GoogleTokens GoogleTokenSource
}

func NewFactory() HTTPFactory {
	return HTTPFactory{Client: &http.Client{Timeout: 180 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, URLClient: safeImageURLClient()}
}
func (f HTTPFactory) Build(config domain.Config) (port.Generator, error) {
	if config.AuthType == "" {
		config.AuthType = domain.DefaultAuthType(config.Protocol)
	}
	if strings.TrimSpace(config.ModelName) == "" {
		return nil, errors.New("image model name is required")
	}
	switch config.AuthType {
	case "", domain.AuthTypeBearer, domain.AuthTypeGoogleAPIKey:
		if strings.TrimSpace(config.APIKey) == "" {
			return nil, errors.New("image model API key is required")
		}
	case domain.AuthTypeGoogleADC:
		if config.Protocol != domain.ProtocolGoogleImagen {
			return nil, errors.New("Google ADC authentication is only supported for Google Imagen")
		}
	case domain.AuthTypeNone:
	default:
		return nil, errors.New("unsupported image model auth type")
	}
	fallback, suffix := "", ""
	switch config.Protocol {
	case domain.ProtocolOpenAIImages:
		fallback, suffix = "https://api.openai.com", "v1/images/generations"
	case domain.ProtocolGeminiNativeImage:
		fallback, suffix = "https://generativelanguage.googleapis.com", "v1beta/models/"+url.PathEscape(config.ModelName)+":generateContent"
	case domain.ProtocolGoogleImagen:
		if _, err := imagenEndpoint(config.BaseURL, config.ModelName); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported image protocol %q", config.Protocol)
	}
	if config.Protocol != domain.ProtocolGoogleImagen {
		if _, err := endpoint(config.BaseURL, fallback, suffix); err != nil {
			return nil, err
		}
	}
	client := f.Client
	if client == nil {
		client = NewFactory().Client
	}
	switch config.Protocol {
	case domain.ProtocolOpenAIImages:
		return &openAIImages{config: config, client: client, urlClient: f.URLClient}, nil
	case domain.ProtocolGeminiNativeImage:
		return &geminiNative{config: config, client: client}, nil
	case domain.ProtocolGoogleImagen:
		tokens := f.GoogleTokens
		if tokens == nil {
			tokens = &defaultGoogleTokenSource{}
		}
		return &googleImagen{config: config, client: client, tokens: tokens}, nil
	default:
		return nil, fmt.Errorf("unsupported image protocol %q", config.Protocol)
	}
}

func safeImageURLClient() *http.Client {
	transport := &http.Transport{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var last error
		for _, candidate := range ips {
			if !candidate.IP.IsGlobalUnicast() || candidate.IP.IsPrivate() || candidate.IP.IsLoopback() || candidate.IP.IsLinkLocalUnicast() || candidate.IP.IsUnspecified() {
				continue
			}
			conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		if last != nil {
			return nil, last
		}
		return nil, errors.New("image URL resolved to a non-public address")
	}
	return &http.Client{Timeout: 45 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func endpoint(base, fallback, suffix string) (string, error) {
	root := strings.TrimRight(strings.TrimSpace(base), "/")
	if root == "" {
		root = fallback
	}
	u, err := url.Parse(root)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("invalid image model base_url")
	}
	basePath := strings.TrimRight(u.Path, "/")
	for strings.HasSuffix(basePath, "/v1") || strings.HasSuffix(basePath, "/v1beta") {
		basePath = strings.TrimSuffix(basePath, "/v1")
		basePath = strings.TrimSuffix(basePath, "/v1beta")
	}
	u.Path = path.Join(basePath, suffix)
	u.RawPath = ""
	return u.String(), nil
}
func auth(req *http.Request, config domain.Config) error {
	switch config.AuthType {
	case "", domain.AuthTypeBearer:
		if config.APIKey == "" {
			return errors.New("image model API key is required")
		}
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	case domain.AuthTypeGoogleAPIKey:
		if config.APIKey == "" {
			return errors.New("image model API key is required")
		}
		req.Header.Set("x-goog-api-key", config.APIKey)
	case domain.AuthTypeNone:
	default:
		return errors.New("unsupported image model auth type")
	}
	return nil
}
func doJSON(ctx context.Context, client *http.Client, method, target string, config domain.Config, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err = auth(req, config); err != nil {
		return nil, err
	}
	return doLimit(req, client, maxJSONResponseBytes)
}
func do(req *http.Request, client *http.Client) ([]byte, error) {
	return doLimit(req, client, maxResponseBytes)
}
func doLimit(req *http.Request, client *http.Client, maxBytes int) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("image model request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("image model request failed (HTTP %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil || len(data) > maxBytes {
		return nil, errors.New("image model response is invalid or too large")
	}
	return data, nil
}
func validateRequest(request port.GenerateRequest) error {
	if strings.TrimSpace(request.Prompt) == "" || len(request.Prompt) > maxPromptBytes {
		return errors.New("image prompt is empty or too large")
	}
	if len(request.ReferenceImages) > 4 {
		return errors.New("at most four reference images are supported")
	}
	for _, value := range []string{request.Size, request.AspectRatio, request.Background, request.Quality, request.OutputFormat} {
		if len(value) > 128 {
			return errors.New("image generation option is too long")
		}
	}
	total := 0
	images := append([]port.ReferenceImage(nil), request.ReferenceImages...)
	if request.Mask != nil {
		images = append(images, *request.Mask)
	}
	for _, reference := range images {
		if len(reference.Data) == 0 || len(reference.Data) > imageasset.MaxSourceBytes {
			return imageasset.ErrTooLarge
		}
		total += len(reference.Data)
		if total > maxReferenceBytes {
			return errors.New("reference images exceed the 32 MiB request limit")
		}
		info, err := imageasset.Inspect(reference.Data)
		if err != nil {
			return err
		}
		if info.MIMEType != reference.MIMEType {
			return errors.New("reference image MIME type does not match its data")
		}
	}
	if request.OutputFormat != "" && request.OutputFormat != "auto" && request.OutputFormat != "png" && request.OutputFormat != "jpeg" && request.OutputFormat != "webp" {
		return errors.New("unsupported image output format")
	}
	return nil
}

func checkedImage(data []byte, mime string) (port.GenerateResult, error) {
	if len(data) == 0 || len(data) > maxResponseBytes {
		return port.GenerateResult{}, errors.New("image model returned an empty or oversized image")
	}
	info, err := imageasset.Inspect(data)
	if err != nil || info.MIMEType != mime {
		return port.GenerateResult{}, errors.New("image model returned an invalid image")
	}
	return port.GenerateResult{Data: data, MIMEType: mime}, nil
}

type openAIImages struct {
	config            domain.Config
	client, urlClient *http.Client
}

func (*openAIImages) SupportsReferenceImages() bool { return true }
func (a *openAIImages) Generate(ctx context.Context, r port.GenerateRequest) (port.GenerateResult, error) {
	if err := validateRequest(r); err != nil {
		return port.GenerateResult{}, err
	}
	base, err := endpoint(a.config.BaseURL, "https://api.openai.com", "v1/images/generations")
	if err != nil {
		return port.GenerateResult{}, err
	}
	if len(r.ReferenceImages) > 0 || r.Mask != nil {
		return a.edit(ctx, r)
	}
	body := map[string]any{"model": a.config.ModelName, "prompt": r.Prompt, "n": 1}
	for key, value := range map[string]string{"size": r.Size, "quality": r.Quality, "background": r.Background, "output_format": r.OutputFormat} {
		if value != "" && value != "auto" {
			body[key] = value
		}
	}
	data, err := doJSON(ctx, a.client, http.MethodPost, base, a.config, body)
	if err != nil {
		return port.GenerateResult{}, err
	}
	var response struct {
		Data []struct {
			B64     string `json:"b64_json"`
			URL     string `json:"url"`
			Revised string `json:"revised_prompt"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &response) != nil || len(response.Data) == 0 {
		return port.GenerateResult{}, errors.New("image model returned an invalid response")
	}
	item := response.Data[0]
	var imageData []byte
	if item.B64 != "" {
		imageData, err = base64.StdEncoding.DecodeString(item.B64)
		if err != nil {
			return port.GenerateResult{}, errors.New("image model returned invalid base64")
		}
	} else if item.URL != "" {
		imageData, err = a.downloadURL(ctx, item.URL)
		if err != nil {
			return port.GenerateResult{}, err
		}
	} else {
		return port.GenerateResult{}, errors.New("image model returned no image")
	}
	format := strings.TrimSpace(r.OutputFormat)
	mime := ""
	switch format {
	case "jpeg":
		mime = "image/jpeg"
	case "png":
		mime = "image/png"
	case "webp":
		mime = "image/webp"
	}
	if mime == "" {
		info, e := imageasset.Inspect(imageData)
		if e != nil {
			return port.GenerateResult{}, e
		}
		mime = info.MIMEType
	}
	result, err := checkedImage(imageData, mime)
	result.RevisedText = item.Revised
	return result, err
}
func (a *openAIImages) edit(ctx context.Context, r port.GenerateRequest) (port.GenerateResult, error) {
	target, err := endpoint(a.config.BaseURL, "https://api.openai.com", "v1/images/edits")
	if err != nil {
		return port.GenerateResult{}, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", a.config.ModelName)
	_ = writer.WriteField("prompt", r.Prompt)
	for key, value := range map[string]string{"size": r.Size, "quality": r.Quality, "background": r.Background, "output_format": r.OutputFormat} {
		if value != "" && value != "auto" {
			_ = writer.WriteField(key, value)
		}
	}
	for _, image := range r.ReferenceImages {
		part, err := createImagePart(writer, "image[]", image)
		if err != nil {
			return port.GenerateResult{}, err
		}
		if _, err = part.Write(image.Data); err != nil {
			return port.GenerateResult{}, err
		}
	}
	if r.Mask != nil {
		part, err := createImagePart(writer, "mask", *r.Mask)
		if err != nil {
			return port.GenerateResult{}, err
		}
		if _, err = part.Write(r.Mask.Data); err != nil {
			return port.GenerateResult{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return port.GenerateResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, &body)
	if err != nil {
		return port.GenerateResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err = auth(req, a.config); err != nil {
		return port.GenerateResult{}, err
	}
	data, err := do(req, a.client)
	if err != nil {
		return port.GenerateResult{}, err
	}
	var response struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &response) != nil || len(response.Data) == 0 {
		return port.GenerateResult{}, errors.New("image model returned an invalid response")
	}
	item := response.Data[0]
	var raw []byte
	if item.B64 != "" {
		raw, err = base64.StdEncoding.DecodeString(item.B64)
	} else {
		raw, err = a.downloadURL(ctx, item.URL)
	}
	if err != nil {
		return port.GenerateResult{}, errors.New("image model returned an invalid image")
	}
	info, err := imageasset.Inspect(raw)
	if err != nil {
		return port.GenerateResult{}, err
	}
	return checkedImage(raw, info.MIMEType)
}
func (a *openAIImages) downloadURL(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return nil, errors.New("image model returned an invalid image URL")
	}
	client := a.urlClient
	if client == nil {
		client = safeImageURLClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	data, err := do(req, client)
	if err != nil {
		return nil, err
	}
	return data, nil
}
func createImagePart(writer *multipart.Writer, field string, image port.ReferenceImage) (io.Writer, error) {
	contentType := strings.TrimSpace(image.MIMEType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": safeFileName(image.FileName)}))
	header.Set("Content-Type", contentType)
	return writer.CreatePart(header)
}

func safeFileName(name string) string {
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	if base == "." || base == "/" || base == "" {
		return "reference.png"
	}
	return base
}

type geminiNative struct {
	config domain.Config
	client *http.Client
}

func (*geminiNative) SupportsReferenceImages() bool { return true }
func (a *geminiNative) Generate(ctx context.Context, r port.GenerateRequest) (port.GenerateResult, error) {
	if (r.Size != "" && r.Size != "auto") || (r.Background != "" && r.Background != "auto") || (r.Quality != "" && r.Quality != "auto") || (r.OutputFormat != "" && r.OutputFormat != "auto" && r.OutputFormat != "png") {
		return port.GenerateResult{}, errors.New("Gemini Native Image supports aspect_ratio only; use OpenAI Images for size, quality, background, or non-PNG output options")
	}
	if err := validateRequest(r); err != nil {
		return port.GenerateResult{}, err
	}
	suffix := "v1beta/models/" + url.PathEscape(a.config.ModelName) + ":generateContent"
	target, err := endpoint(a.config.BaseURL, "https://generativelanguage.googleapis.com", suffix)
	if err != nil {
		return port.GenerateResult{}, err
	}
	parts := make([]any, 0, len(r.ReferenceImages)+2)
	appendRef := func(ref port.ReferenceImage) {
		parts = append(parts, map[string]any{"inlineData": map[string]string{"mimeType": ref.MIMEType, "data": base64.StdEncoding.EncodeToString(ref.Data)}})
	}
	for _, ref := range r.ReferenceImages {
		appendRef(ref)
	}
	if r.Mask != nil {
		appendRef(*r.Mask)
	}
	parts = append(parts, map[string]string{"text": r.Prompt})
	config := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
	imageConfig := map[string]any{}
	if r.AspectRatio != "" {
		imageConfig["aspectRatio"] = r.AspectRatio
	}
	if len(imageConfig) > 0 {
		config["imageConfig"] = imageConfig
	}
	body := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": parts}}, "generationConfig": config}
	data, err := doJSON(ctx, a.client, http.MethodPost, target, a.config, body)
	if err != nil {
		return port.GenerateResult{}, err
	}
	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text   string `json:"text"`
					Inline struct {
						MIME string `json:"mimeType"`
						Data string `json:"data"`
					} `json:"inlineData"`
					InlineSnake struct {
						MIME string `json:"mime_type"`
						Data string `json:"data"`
					} `json:"inline_data"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &response) != nil || len(response.Candidates) == 0 {
		return port.GenerateResult{}, errors.New("image model returned an invalid response")
	}
	var revised string
	for _, part := range response.Candidates[0].Content.Parts {
		if part.Text != "" {
			revised += part.Text
		}
		inline := part.Inline
		if inline.Data == "" {
			inline = struct {
				MIME string `json:"mimeType"`
				Data string `json:"data"`
			}{part.InlineSnake.MIME, part.InlineSnake.Data}
		}
		if inline.Data != "" {
			raw, err := base64.StdEncoding.DecodeString(inline.Data)
			if err != nil {
				return port.GenerateResult{}, errors.New("image model returned invalid base64")
			}
			result, err := checkedImage(raw, inline.MIME)
			result.RevisedText = revised
			return result, err
		}
	}
	return port.GenerateResult{}, errors.New("image model returned no image")
}

func imagenEndpoint(baseURL, model string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", errors.New("google-imagen requires base_url; set the Vertex AI resource root ending in /v1/projects/{project}/locations/{location}")
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("invalid Imagen base_url")
	}
	if model == "" {
		return "", errors.New("invalid Imagen model name")
	}
	for _, char := range model {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') {
			return "", errors.New("invalid Imagen model name")
		}
	}
	root := strings.TrimRight(u.Path, "/")
	projectsAt := strings.Index(root, "/projects/")
	if projectsAt < 0 {
		return "", errors.New("Google Imagen requires a Vertex AI resource root containing project and location")
	}
	resourceParts := strings.Split(root[projectsAt+len("/projects/"):], "/")
	if len(resourceParts) != 3 || resourceParts[0] == "" || resourceParts[1] != "locations" || resourceParts[2] == "" {
		return "", errors.New("Imagen Vertex AI base_url must end with /v1/projects/{project}/locations/{location}")
	}
	prefix := strings.TrimRight(root[:projectsAt], "/")
	if !strings.HasSuffix(prefix, "/v1") {
		prefix += "/v1"
	}
	root = prefix + root[projectsAt:]
	u.Path = root + "/publishers/google/models/" + url.PathEscape(model) + ":predict"
	u.RawPath = ""
	return u.String(), nil
}

type googleImagen struct {
	config domain.Config
	client *http.Client
	tokens GoogleTokenSource
}

func (*googleImagen) SupportsReferenceImages() bool { return false }
func (a *googleImagen) Generate(ctx context.Context, r port.GenerateRequest) (port.GenerateResult, error) {
	if (r.Size != "" && r.Size != "auto") || (r.Background != "" && r.Background != "auto") || (r.Quality != "" && r.Quality != "auto") {
		return port.GenerateResult{}, errors.New("Google Imagen does not support requested size, quality, or background options")
	}
	if len(r.ReferenceImages) > 0 || r.Mask != nil {
		return port.GenerateResult{}, errors.New("Google Imagen does not support reference images; use OpenAI Images or Gemini Native Image")
	}
	if err := validateRequest(r); err != nil {
		return port.GenerateResult{}, err
	}
	target, err := imagenEndpoint(a.config.BaseURL, a.config.ModelName)
	if err != nil {
		return port.GenerateResult{}, err
	}
	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg"}[r.OutputFormat]
	if mime == "" && r.OutputFormat != "" && r.OutputFormat != "auto" {
		return port.GenerateResult{}, errors.New("Google Imagen supports PNG or JPEG output")
	}
	if mime == "" {
		mime = "image/png"
	}
	params := map[string]any{"sampleCount": 1, "outputOptions": map[string]string{"mimeType": mime}}
	if r.AspectRatio != "" {
		params["aspectRatio"] = r.AspectRatio
	}
	config := a.config
	if config.AuthType == domain.AuthTypeGoogleADC {
		if a.tokens == nil {
			return port.GenerateResult{}, errors.New("Google Application Default Credentials are unavailable")
		}
		config.APIKey, err = a.tokens.AccessToken(ctx)
		if err != nil || strings.TrimSpace(config.APIKey) == "" {
			return port.GenerateResult{}, errors.New("Google Application Default Credentials token is unavailable")
		}
		config.AuthType = domain.AuthTypeBearer
	}
	data, err := doJSON(ctx, a.client, http.MethodPost, target, config, map[string]any{"instances": []any{map[string]string{"prompt": r.Prompt}}, "parameters": params})
	if err != nil {
		return port.GenerateResult{}, err
	}
	var response struct {
		Predictions []struct {
			Bytes string `json:"bytesBase64Encoded"`
			MIME  string `json:"mimeType"`
		} `json:"predictions"`
	}
	if json.Unmarshal(data, &response) != nil || len(response.Predictions) == 0 {
		return port.GenerateResult{}, errors.New("image model returned an invalid response")
	}
	raw, err := base64.StdEncoding.DecodeString(response.Predictions[0].Bytes)
	if err != nil {
		return port.GenerateResult{}, errors.New("image model returned invalid base64")
	}
	return checkedImage(raw, response.Predictions[0].MIME)
}
