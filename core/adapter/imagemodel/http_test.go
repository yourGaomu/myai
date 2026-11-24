package imagemodel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "myai/core/domain/imagemodel"
	port "myai/core/port/imagemodel"
)

func pngFixture(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 20, G: 30, B: 40, A: 255})
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func responseJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
func testConfig(serverURL string, protocol domain.Protocol) domain.Config {
	return domain.Config{ID: "test", Name: "test", Protocol: protocol, AuthType: domain.AuthTypeBearer, APIKey: "secret", BaseURL: serverURL, ModelName: "model"}
}

func TestOpenAIImagesGenerationAndBaseURLNormalization(t *testing.T) {
	fixture := pngFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/v1/images/generations" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization missing")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "model" || body["prompt"] != "test prompt" || body["size"] != "1024x1024" || body["output_format"] != "png" {
			t.Errorf("unexpected body: %#v", body)
		}
		responseJSON(t, w, map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(fixture), "revised_prompt": "revised"}}})
	}))
	defer server.Close()
	config := testConfig(server.URL+"/proxy/v1", domain.ProtocolOpenAIImages)
	generator := &openAIImages{config: config, client: server.Client()}
	result, err := generator.Generate(context.Background(), port.GenerateRequest{Prompt: "test prompt", Size: "1024x1024", OutputFormat: "png"})
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/png" || !bytes.Equal(result.Data, fixture) || result.RevisedText != "revised" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestOpenAIImagesEditBuildsMultipartAndMask(t *testing.T) {
	fixture := pngFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Errorf("unexpected edit route %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("authorization missing")
		}
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Fatalf("multipart header %q %v", mediaType, err)
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		seen := map[string]int{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
			seen[part.FormName()]++
			if part.FormName() == "image[]" || part.FormName() == "mask" {
				if !bytes.Equal(data, fixture) {
					t.Errorf("bad %s data", part.FormName())
				}
				if part.Header.Get("Content-Type") != "image/png" {
					t.Errorf("unexpected %s content type: %q", part.FormName(), part.Header.Get("Content-Type"))
				}
			}
		}
		if seen["model"] != 1 || seen["prompt"] != 1 || seen["image[]"] != 1 || seen["mask"] != 1 || seen["quality"] != 1 || seen["output_format"] != 1 {
			t.Errorf("missing multipart fields: %#v", seen)
		}
		responseJSON(t, w, map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(fixture)}}})
	}))
	defer server.Close()
	config := testConfig(server.URL, domain.ProtocolOpenAIImages)
	generator := &openAIImages{config: config, client: server.Client()}
	ref := port.ReferenceImage{Data: fixture, MIMEType: "image/png", FileName: "ref.png"}
	result, err := generator.Generate(context.Background(), port.GenerateRequest{Prompt: "edit", ReferenceImages: []port.ReferenceImage{ref}, Mask: &ref, Quality: "high", OutputFormat: "png"})
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/png" {
		t.Fatalf("mime %s", result.MIMEType)
	}
}

func TestOpenAIImageURLDownloadDoesNotForwardAuthAndRejectsRedirect(t *testing.T) {
	fixture := pngFixture(t)
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("provider authorization leaked to image host")
		}
		w.Write(fixture)
	}))
	defer imageServer.Close()
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, imageServer.URL, http.StatusFound) }))
	defer redirectServer.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseJSON(t, w, map[string]any{"data": []any{map[string]string{"url": imageServer.URL}}})
	}))
	defer provider.Close()
	generator := &openAIImages{config: testConfig(provider.URL, domain.ProtocolOpenAIImages), client: provider.Client(), urlClient: imageServer.Client()}
	result, err := generator.Generate(context.Background(), port.GenerateRequest{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Data, fixture) {
		t.Fatal("downloaded bytes changed")
	}
	noRedirect := redirectServer.Client()
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	generator.urlClient = noRedirect
	if _, err := generator.downloadURL(context.Background(), redirectServer.URL); err == nil {
		t.Fatal("followed redirect")
	}
}

func TestGeminiNativeImageRequestAndResponse(t *testing.T) {
	fixture := pngFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/v1beta/models/gemini-image:generateContent" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "secret" {
			t.Error("Google API key header missing")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		contents := payload["contents"].([]any)
		parts := contents[0].(map[string]any)["parts"].([]any)
		if len(parts) != 2 || parts[0].(map[string]any)["inlineData"] == nil || parts[1].(map[string]any)["text"] != "describe image" {
			t.Errorf("bad parts %#v", parts)
		}
		generation := payload["generationConfig"].(map[string]any)
		if generation["imageConfig"].(map[string]any)["aspectRatio"] != "3:4" {
			t.Error("aspect ratio missing")
		}
		responseJSON(t, w, map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]string{"text": "ok"}, map[string]any{"inlineData": map[string]string{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(fixture)}}}}}}})
	}))
	defer server.Close()
	config := testConfig(server.URL+"/gateway/v1beta", domain.ProtocolGeminiNativeImage)
	config.ModelName = "gemini-image"
	config.AuthType = domain.AuthTypeGoogleAPIKey
	generator := &geminiNative{config: config, client: server.Client()}
	ref := port.ReferenceImage{Data: fixture, MIMEType: "image/png", FileName: "r.png"}
	result, err := generator.Generate(context.Background(), port.GenerateRequest{Prompt: "describe image", ReferenceImages: []port.ReferenceImage{ref}, AspectRatio: "3:4"})
	if err != nil {
		t.Fatal(err)
	}
	if result.RevisedText != "ok" || result.MIMEType != "image/png" {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestGoogleImagenPredictAndRejectsReferences(t *testing.T) {
	fixture := pngFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/demo/locations/us-central1/publishers/google/models/imagen:predict" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing Vertex bearer auth")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload["instances"].([]any)) != 1 || payload["parameters"].(map[string]any)["sampleCount"] != float64(1) {
			t.Errorf("bad request %#v", payload)
		}
		responseJSON(t, w, map[string]any{"predictions": []any{map[string]string{"bytesBase64Encoded": base64.StdEncoding.EncodeToString(fixture), "mimeType": "image/png"}}})
	}))
	defer server.Close()
	config := testConfig(server.URL+"/v1/projects/demo/locations/us-central1", domain.ProtocolGoogleImagen)
	config.ModelName = "imagen"
	config.AuthType = domain.AuthTypeBearer
	generator := &googleImagen{config: config, client: server.Client()}
	result, err := generator.Generate(context.Background(), port.GenerateRequest{Prompt: "simple"})
	if err != nil || result.MIMEType != "image/png" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	_, err = generator.Generate(context.Background(), port.GenerateRequest{Prompt: "simple", ReferenceImages: []port.ReferenceImage{{Data: fixture}}})
	if err == nil || !strings.Contains(err.Error(), "does not support reference images") {
		t.Fatalf("unexpected reference error %v", err)
	}
}

func TestImagenEndpointRequiresProjectAndPreservesProxyPrefix(t *testing.T) {
	if _, err := imagenEndpoint("", "imagen-4.0-generate-001"); err == nil {
		t.Fatal("expected explicit Vertex resource root")
	}
	got, err := imagenEndpoint("https://vertex.example.test/proxy/v1/projects/demo/locations/us-central1", "imagen-4.0-generate-001")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://vertex.example.test/proxy/v1/projects/demo/locations/us-central1/publishers/google/models/imagen-4.0-generate-001:predict" {
		t.Fatalf("unexpected endpoint %s", got)
	}
	if _, err := imagenEndpoint("https://aiplatform.googleapis.com/v1", "imagen-4.0-generate-001"); err == nil {
		t.Fatal("expected project/location to be required")
	}
}

type staticGoogleTokenSource struct{ token string }

func (s staticGoogleTokenSource) AccessToken(context.Context) (string, error) { return s.token, nil }

func TestGoogleImagenUsesADCRefreshableBearerCredentials(t *testing.T) {
	fixture := pngFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer refreshed-access-token" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		responseJSON(t, w, map[string]any{"predictions": []any{map[string]string{"bytesBase64Encoded": base64.StdEncoding.EncodeToString(fixture), "mimeType": "image/png"}}})
	}))
	defer server.Close()
	config := testConfig(server.URL+"/v1/projects/demo/locations/us-central1", domain.ProtocolGoogleImagen)
	config.AuthType = ""
	config.APIKey = ""
	factory := HTTPFactory{Client: server.Client(), GoogleTokens: staticGoogleTokenSource{token: "refreshed-access-token"}}
	generator, err := factory.Build(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := generator.Generate(context.Background(), port.GenerateRequest{Prompt: "small square"})
	if err != nil || result.MIMEType != "image/png" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
