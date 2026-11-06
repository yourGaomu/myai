package shortener_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	shortener "myai-url-shortener/internal/shortener"
	"myai-url-shortener/internal/shortener/handle"
	"myai-url-shortener/internal/shortener/objectstore"
	service2 "myai-url-shortener/internal/shortener/service"
	"myai-url-shortener/internal/shortener/store"
	"myai-url-shortener/internal/shortener/store/memoryStore"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const handlerTestAPIKey = "handler-test-api-key"

func TestHandlerCreateAndRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{
		BaseURL:    "http://short.local",
		DefaultTTL: time.Hour,
	})
	router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey})

	body, err := json.Marshal(shortener.CreateLinkRequest{URL: "https://example.com/a.png"})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	createRequest := httptest.NewRequest(http.MethodPost, "/api/links", bytes.NewReader(body))
	createRequest.Header.Set("Authorization", "Bearer "+handlerTestAPIKey)
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, createResponse.Code, createResponse.Body.String())
	}

	var payload shortener.CreateLinkResponse
	if err := json.NewDecoder(createResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	redirectRequest := httptest.NewRequest(http.MethodGet, "/s/"+payload.Code, nil)
	redirectResponse := httptest.NewRecorder()
	router.ServeHTTP(redirectResponse, redirectRequest)
	if redirectResponse.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, redirectResponse.Code)
	}
	if got := redirectResponse.Header().Get("Location"); got != "https://example.com/a.png" {
		t.Fatalf("unexpected location: %s", got)
	}

	for _, path := range []string{"/api/links/" + payload.Code, "/api/links"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "bearer "+handlerTestAPIKey)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), payload.Code) {
			t.Fatalf("authenticated read failed: status %d", response.Code)
		}
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/links/"+payload.Code, nil)
	deleteRequest.Header.Set("Authorization", "Bearer "+handlerTestAPIKey)
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("authenticated delete failed: status %d", deleteResponse.Code)
	}
	if _, err := service.GetLinkInfo(context.Background(), payload.Code); !errors.Is(err, store.ErrLinkNotFound) {
		t.Fatalf("deleted link should be hidden, got %v", err)
	}
	redirectResponse = httptest.NewRecorder()
	router.ServeHTTP(redirectResponse, httptest.NewRequest(http.MethodGet, "/s/"+payload.Code, nil))
	if redirectResponse.Code != http.StatusNotFound {
		t.Fatalf("deleted link should not redirect, got %d", redirectResponse.Code)
	}
}

func TestHandlerManagementRequiresBearerKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	objects := &handlerObjectStore{}
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{ObjectStore: objects})
	link, err := service.CreateLink(context.Background(), shortener.CreateLinkRequest{URL: "https://example.com/private"})
	if err != nil {
		t.Fatalf("seed link: %v", err)
	}
	router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey, MaxUploadBytes: 1024})

	authCases := []struct {
		name   string
		values []string
	}{
		{name: "missing"},
		{name: "empty", values: []string{""}},
		{name: "empty bearer", values: []string{"Bearer "}},
		{name: "missing token", values: []string{"Bearer"}},
		{name: "wrong scheme", values: []string{"Basic " + handlerTestAPIKey}},
		{name: "raw key", values: []string{handlerTestAPIKey}},
		{name: "wrong key same length", values: []string{"Bearer " + strings.Repeat("x", len(handlerTestAPIKey))}},
		{name: "wrong key different length", values: []string{"Bearer wrong-test-key"}},
		{name: "extra whitespace", values: []string{"Bearer  " + handlerTestAPIKey}},
		{name: "extra token", values: []string{"Bearer " + handlerTestAPIKey + " extra"}},
		{name: "duplicate", values: []string{"Bearer " + handlerTestAPIKey, "Bearer " + handlerTestAPIKey}},
		{name: "conflicting duplicate", values: []string{"Bearer " + handlerTestAPIKey, "Bearer wrong-test-key"}},
		{name: "merged headers", values: []string{"Bearer " + handlerTestAPIKey + ", Bearer " + handlerTestAPIKey}},
	}
	for _, route := range protectedHandlerRoutes(link.Code) {
		for _, auth := range authCases {
			t.Run(route.name+"/"+auth.name, func(t *testing.T) {
				body := &handlerReadTracker{Reader: strings.NewReader("unparsed request body")}
				request := httptest.NewRequest(route.method, route.path, body)
				// Authentication must run even before the known-size upload rejection.
				request.ContentLength = 2048
				for _, value := range auth.values {
					request.Header.Add("Authorization", value)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				assertHandlerUnauthorized(t, response)
				if body.reads != 0 {
					t.Fatal("unauthenticated request body was read")
				}
			})
		}
	}
	links, err := service.ListLinks(context.Background())
	if err != nil || len(links) != 1 || links[0].Code != link.Code || links[0].Visits != 0 {
		t.Fatal("unauthenticated management requests changed link state")
	}
	if objects.uploads != 0 {
		t.Fatal("unauthenticated upload reached object storage")
	}

	request := httptest.NewRequest(http.MethodPost, "/api/links?api_key="+handlerTestAPIKey, nil)
	request.Header.Set("X-API-Key", handlerTestAPIKey)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assertHandlerUnauthorized(t, response)
}

func TestHandlerUnconfiguredKeyFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{})
	link, err := service.CreateLink(context.Background(), shortener.CreateLinkRequest{URL: "https://example.com/public"})
	if err != nil {
		t.Fatalf("seed link: %v", err)
	}
	for _, apiKey := range []string{"", " ", "\t\r\n"} {
		router := handle.NewRouter(service, handle.RouterOptions{APIKey: apiKey})
		for _, route := range protectedHandlerRoutes(link.Code) {
			for _, authorization := range []string{"", "Bearer ", "Bearer " + handlerTestAPIKey} {
				request := httptest.NewRequest(route.method, route.path, nil)
				request.Header.Set("Authorization", authorization)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				assertHandlerUnauthorized(t, response)
			}
		}
		for path, status := range map[string]int{"/health": http.StatusOK, "/s/" + link.Code: http.StatusFound} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != status {
				t.Fatalf("public route %s: expected %d, got %d", path, status, response.Code)
			}
		}
	}
}

func TestHandlerAuthenticatedURLValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{})
	router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey})
	for _, body := range []string{`{"url":"relative/path"}`, `{"url":""}`, `invalid json`} {
		request := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+handlerTestAPIKey)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid URL request: expected 400, got %d", response.Code)
		}
	}
	links, err := service.ListLinks(context.Background())
	if err != nil || len(links) != 0 {
		t.Fatal("invalid URL request created a link")
	}
}

func TestHandlerPublicRedirectHonorsLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	linkStore := memoryStore.NewMemoryStore()
	expired := time.Now().Add(-time.Hour)
	for _, link := range []shortener.Link{
		{Code: "expired", URL: "https://example.com/expired", ExpiresAt: &expired},
		{Code: "exhausted", URL: "https://example.com/exhausted", MaxVisits: 1, Visits: 1},
	} {
		if err := linkStore.Create(context.Background(), link); err != nil {
			t.Fatalf("seed link: %v", err)
		}
	}
	service := service2.NewService(linkStore, service2.ServiceOptions{})
	router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey})
	for _, code := range []string{"expired", "exhausted"} {
		request := httptest.NewRequest(http.MethodGet, "/s/"+code, nil)
		request.Header.Set("Authorization", "Bearer wrong-test-key")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusGone || response.Header().Get("Location") != "" {
			t.Fatalf("unavailable public link redirected: status %d", response.Code)
		}
	}
}

func TestHandlerCORSPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := handle.NewRouter(nil, handle.RouterOptions{APIKey: handlerTestAPIKey})
	request := httptest.NewRequest(http.MethodOptions, "/api/assets", nil)
	request.Header.Set("Origin", "https://client.example.com")
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !strings.Contains(response.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("authenticated browser preflight failed: status %d", response.Code)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets", nil))
	assertHandlerUnauthorized(t, response)
}

func TestHandlerAuthenticationDoesNotLogKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	router := handle.NewRouter(nil, handle.RouterOptions{APIKey: handlerTestAPIKey})
	request := httptest.NewRequest(http.MethodPost, "/api/assets", nil)
	request.Header.Set("Authorization", "Bearer rejected-test-key")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assertHandlerUnauthorized(t, response)
	for _, value := range []string{handlerTestAPIKey, "rejected-test-key"} {
		if strings.Contains(output.String(), value) || strings.Contains(response.Body.String(), value) {
			t.Fatal("authentication exposed a key in logs or response")
		}
	}
}

func TestHandlerUploadRequestLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	smallBody, contentType := handlerMultipartBody(t, 32, 0, 0)
	largeFile, _ := handlerMultipartBody(t, 4096, 0, 0)
	largeField, _ := handlerMultipartBody(t, 32, 4096, 0)
	multipleFiles, _ := handlerMultipartBody(t, 600, 0, 600)
	largeEpilogue := append(append([]byte(nil), smallBody...), bytes.Repeat([]byte("e"), 8192)...)

	tests := []struct {
		name          string
		body          []byte
		maxBytes      int64
		contentLength int64
		wantStatus    int
	}{
		{name: "within limit", body: smallBody, maxBytes: 1024, contentLength: int64(len(smallBody)), wantStatus: http.StatusCreated},
		{name: "exact request boundary", body: smallBody, maxBytes: int64(len(smallBody)), contentLength: int64(len(smallBody)), wantStatus: http.StatusCreated},
		{name: "known size including overhead", body: smallBody, maxBytes: int64(len(smallBody) - 1), contentLength: int64(len(smallBody)), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "unknown size including overhead", body: smallBody, maxBytes: int64(len(smallBody) - 1), contentLength: -1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "large file without length", body: largeFile, maxBytes: 1024, contentLength: -1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "large field without length", body: largeField, maxBytes: 1024, contentLength: -1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "multiple files combined", body: multipleFiles, maxBytes: 1024, contentLength: -1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "declared length too small", body: largeFile, maxBytes: 1024, contentLength: 1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "zero declared length", body: largeFile, maxBytes: 1024, contentLength: 0, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "epilogue counts toward limit", body: largeEpilogue, maxBytes: 1024, contentLength: -1, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "zero router option uses safe default", body: smallBody, maxBytes: 0, contentLength: -1, wantStatus: http.StatusCreated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects := &handlerObjectStore{}
			service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{ObjectStore: objects})
			router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey, MaxUploadBytes: test.maxBytes})
			body := &handlerReadTracker{Reader: bytes.NewReader(test.body)}
			request := httptest.NewRequest(http.MethodPost, "/api/assets", body)
			request.Header.Set("Authorization", "Bearer "+handlerTestAPIKey)
			request.Header.Set("Content-Type", contentType)
			request.ContentLength = test.contentLength
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
			links, err := service.ListLinks(context.Background())
			if err != nil {
				t.Fatalf("list links: %v", err)
			}
			if test.wantStatus == http.StatusCreated {
				if objects.uploads != 1 || len(links) != 1 || string(objects.data) != strings.Repeat("f", 32) {
					t.Fatal("successful upload did not preserve file content and link creation")
				}
				if links[0].Title != "test upload" || links[0].Scope != "test-session" || links[0].MaxVisits != 3 || objects.request.FileName != "demo.txt" {
					t.Fatal("successful upload did not preserve multipart metadata")
				}
				var payload shortener.CreateObjectLinkResponse
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Size != 32 || payload.ShortURL == "" {
					t.Fatal("upload response is incompatible")
				}
				redirectResponse := httptest.NewRecorder()
				router.ServeHTTP(redirectResponse, httptest.NewRequest(http.MethodGet, "/s/"+payload.Code, nil))
				if redirectResponse.Code != http.StatusFound || redirectResponse.Header().Get("Location") != "https://assets.example.com/demo.txt" {
					t.Fatal("uploaded asset short link must remain publicly accessible")
				}
			} else {
				if objects.uploads != 0 || len(links) != 0 {
					t.Fatal("oversized request wrote an object or link")
				}
				if body.bytesRead > test.maxBytes+1 {
					t.Fatalf("read past upload limit: %d bytes", body.bytesRead)
				}
				if test.contentLength > test.maxBytes && body.reads != 0 {
					t.Fatal("known oversized request should be rejected before body parsing")
				}
			}
		})
	}
}

func TestHandlerChunkedUploadLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fileSize := range []int{32, 4096} {
		body, contentType := handlerMultipartBody(t, fileSize, 0, 0)
		objects := &handlerObjectStore{}
		service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{ObjectStore: objects})
		router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey, MaxUploadBytes: 1024})
		sawChunked := make(chan bool, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawChunked <- r.ContentLength == -1 && len(r.TransferEncoding) == 1 && r.TransferEncoding[0] == "chunked"
			router.ServeHTTP(w, r)
		}))
		func() {
			defer server.Close()
			request, err := http.NewRequest(http.MethodPost, server.URL+"/api/assets", io.NopCloser(bytes.NewReader(body)))
			if err != nil {
				t.Fatalf("build chunked request: %v", err)
			}
			request.Header.Set("Authorization", "Bearer "+handlerTestAPIKey)
			request.Header.Set("Content-Type", contentType)
			client := server.Client()
			client.Timeout = 5 * time.Second
			response, err := client.Do(request)
			if err != nil {
				t.Fatalf("chunked upload: %v", err)
			}
			defer response.Body.Close()
			if !<-sawChunked {
				t.Fatal("test request did not arrive as a chunked upload without Content-Length")
			}
			wantStatus := http.StatusCreated
			if fileSize > 1024 {
				wantStatus = http.StatusRequestEntityTooLarge
			}
			if response.StatusCode != wantStatus {
				t.Fatalf("chunked upload: expected %d, got %d", wantStatus, response.StatusCode)
			}
			links, err := service.ListLinks(context.Background())
			if err != nil {
				t.Fatalf("list links: %v", err)
			}
			if wantStatus == http.StatusRequestEntityTooLarge && (objects.uploads != 0 || len(links) != 0) {
				t.Fatal("oversized chunked upload wrote an object or link")
			}
		}()
	}
}

func TestHandlerUploadCleansTemporaryFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	body, contentType := handlerMultipartBody(t, 8192, 0, 0)
	for _, tooLarge := range []bool{false, true} {
		objects := &handlerObjectStore{}
		service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{ObjectStore: objects})
		router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey, MaxUploadBytes: int64(len(body) + 512)})
		router.MaxMultipartMemory = 1
		payload := append([]byte(nil), body...)
		if tooLarge {
			payload = append(payload, bytes.Repeat([]byte("e"), 8192)...)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/assets", bytes.NewReader(payload))
		request.ContentLength = -1
		request.Header.Set("Authorization", "Bearer "+handlerTestAPIKey)
		request.Header.Set("Content-Type", contentType)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		wantStatus := http.StatusCreated
		if tooLarge {
			wantStatus = http.StatusRequestEntityTooLarge
		}
		if response.Code != wantStatus {
			t.Fatalf("temporary file upload: expected %d, got %d", wantStatus, response.Code)
		}
		files, err := os.ReadDir(tempDir)
		if err != nil || len(files) != 0 {
			t.Fatal("multipart temporary files were not removed")
		}
		if tooLarge && objects.uploads != 0 {
			t.Fatal("oversized multipart epilogue reached storage")
		}
	}
}

func TestHandlerMalformedUploadDoesNotWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	objects := &handlerObjectStore{}
	service := service2.NewService(memoryStore.NewMemoryStore(), service2.ServiceOptions{ObjectStore: objects})
	router := handle.NewRouter(service, handle.RouterOptions{APIKey: handlerTestAPIKey})
	body, contentType := handlerMultipartBody(t, 32, 0, 0)
	for _, payload := range [][]byte{[]byte("invalid multipart"), body[:len(body)-20]} {
		request := httptest.NewRequest(http.MethodPost, "/api/assets", bytes.NewReader(payload))
		request.Header.Set("Authorization", "Bearer "+handlerTestAPIKey)
		request.Header.Set("Content-Type", contentType)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || objects.uploads != 0 {
			t.Fatalf("malformed upload was not rejected before storage: status %d", response.Code)
		}
	}
}

func protectedHandlerRoutes(code string) []struct{ name, method, path string } {
	return []struct{ name, method, path string }{
		{"create", http.MethodPost, "/api/links"},
		{"upload", http.MethodPost, "/api/assets"},
		{"details", http.MethodGet, "/api/links/" + code},
		{"list", http.MethodGet, "/api/links"},
		{"delete", http.MethodDelete, "/api/links/" + code},
	}
}

func assertHandlerUnauthorized(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusUnauthorized || response.Body.String() != `{"error":"unauthorized"}` || response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("expected generic 401 Bearer challenge, got status %d", response.Code)
	}
}

func handlerMultipartBody(t *testing.T, fileSize, fieldSize, extraFileSize int) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.SetBoundary("handler-test-boundary"); err != nil {
		t.Fatalf("set multipart boundary: %v", err)
	}
	part, err := writer.CreateFormFile("file", "demo.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := io.WriteString(part, strings.Repeat("f", fileSize)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if extraFileSize > 0 {
		part, err := writer.CreateFormFile("extra", "other.txt")
		if err != nil {
			t.Fatalf("create extra multipart file: %v", err)
		}
		if _, err := io.WriteString(part, strings.Repeat("x", extraFileSize)); err != nil {
			t.Fatalf("write extra multipart file: %v", err)
		}
	}
	for key, value := range map[string]string{
		"title": "test upload", "scope": "test-session", "max_visits": "3", "ttl_seconds": "600",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	if fieldSize > 0 {
		if err := writer.WriteField("extra", strings.Repeat("x", fieldSize)); err != nil {
			t.Fatalf("write extra field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

type handlerReadTracker struct {
	io.Reader
	reads     int
	bytesRead int64
}

func (r *handlerReadTracker) Read(p []byte) (int, error) {
	r.reads++
	n, err := r.Reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

type handlerObjectStore struct {
	uploads int
	data    []byte
	request objectstore.UploadRequest
}

func (s *handlerObjectStore) Upload(_ context.Context, request objectstore.UploadRequest) (objectstore.ObjectInfo, error) {
	s.uploads++
	s.request = request
	data, err := io.ReadAll(request.Reader)
	if err != nil {
		return objectstore.ObjectInfo{}, err
	}
	s.data = data
	return objectstore.ObjectInfo{
		Bucket: "test-assets", Key: "uploads/demo.txt", FileName: request.FileName,
		ContentType: request.ContentType, Size: int64(len(data)),
	}, nil
}

func (s *handlerObjectStore) PresignedGetURL(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return "https://assets.example.com/demo.txt", nil
}
