package asset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestUploadFile(t *testing.T) {
	var gotTitle string
	var gotTTL string
	var gotFileName string
	var gotContent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-asset-key" {
			t.Error("upload did not include the configured Bearer API key")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.RawQuery != "" {
			t.Error("upload credentials must not be sent in the query string")
		}
		if r.URL.Path != "/api/assets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		gotTitle = r.FormValue("title")
		gotTTL = r.FormValue("ttl_seconds")

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}
		defer file.Close()
		gotFileName = header.Filename

		buffer := new(strings.Builder)
		if _, err := io.Copy(buffer, file); err != nil {
			t.Fatalf("read upload content: %v", err)
		}
		gotContent = buffer.String()

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(UploadFileResponse{
			Code:        "abc123",
			ShortURL:    serverURL(r) + "/s/abc123",
			FileName:    header.Filename,
			ContentType: "text/plain",
			Size:        int64(len(gotContent)),
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:           server.URL,
		APIKey:            "  test-asset-key  ",
		DefaultTTLSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	response, err := client.UploadFile(context.Background(), UploadFileRequest{
		Reader:   strings.NewReader("hello"),
		FileName: "hello.txt",
		Title:    "hello file",
	})
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}

	if response.Code != "abc123" {
		t.Fatalf("unexpected code: %s", response.Code)
	}
	if gotTitle != "hello file" {
		t.Fatalf("unexpected title: %s", gotTitle)
	}
	if gotTTL != "3600" {
		t.Fatalf("unexpected ttl: %s", gotTTL)
	}
	if gotFileName != "hello.txt" {
		t.Fatalf("unexpected file name: %s", gotFileName)
	}
	if gotContent != "hello" {
		t.Fatalf("unexpected content: %s", gotContent)
	}
}

func TestNewClientRejectsInvalidBaseURL(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "ftp://example.com"}); err == nil {
		t.Fatal("expected invalid scheme error")
	}
}

func TestCodeFromShortURLRequiresConfiguredHost(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "http://short.local/base"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	code, err := client.codeFromShortURL("http://short.local/base/s/abc123")
	if err != nil {
		t.Fatalf("code from short url: %v", err)
	}
	if code != "abc123" {
		t.Fatalf("unexpected code: %s", code)
	}

	if _, err := client.codeFromShortURL("http://evil.local/base/s/abc123"); err == nil {
		t.Fatal("expected host validation error")
	}
}

func TestMissingAPIKeyFailsOnlyWhenUsed(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: " \t "})
	if err != nil {
		t.Fatalf("missing API key must not prevent client initialization: %v", err)
	}
	if got := client.ShortURL("abc123"); got != server.URL+"/s/abc123" {
		t.Fatalf("unexpected short URL: %s", got)
	}

	checks := map[string]func() error{
		"upload": func() error {
			_, err := client.UploadFile(context.Background(), UploadFileRequest{Reader: strings.NewReader("hello"), FileName: "hello.txt"})
			return err
		},
		"metadata": func() error {
			_, err := client.GetLinkInfo(context.Background(), "abc123")
			return err
		},
		"download metadata": func() error {
			_, err := client.DownloadAsset(context.Background(), DownloadAssetRequest{Code: "abc123"})
			return err
		},
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			if err := check(); err == nil || !strings.Contains(err.Error(), "ASSET_API_KEY") {
				t.Fatalf("expected actionable missing-key error, got %v", err)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("missing key sent %d network requests", requests)
	}
}

func TestDownloadAssetAuthenticatesOnlyMetadata(t *testing.T) {
	var metadataRequests, publicRequests, objectRequests int
	objectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectRequests++
		if r.Header.Get("Authorization") != "" {
			t.Error("public object download leaked an Authorization header")
		}
		_, _ = io.WriteString(w, "hello")
	}))
	defer objectServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/links/abc123":
			metadataRequests++
			if r.Header.Get("Authorization") != "Bearer test-asset-key" {
				t.Error("metadata did not include the configured Bearer API key")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(LinkInfo{
				Code: "abc123", Kind: "object", ObjectFileName: "hello.txt", ObjectContentType: "text/plain", ObjectSize: 5,
			})
		case "/s/abc123":
			publicRequests++
			if r.Header.Get("Authorization") != "" {
				t.Error("public short link leaked an Authorization header")
			}
			http.Redirect(w, r, "/public-object", http.StatusFound)
		case "/public-object":
			publicRequests++
			if r.Header.Get("Authorization") != "" {
				t.Error("same-origin public redirect leaked an Authorization header")
			}
			http.Redirect(w, r, objectServer.URL+"/hello.txt", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-asset-key"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	response, err := client.DownloadAsset(context.Background(), DownloadAssetRequest{URL: server.URL + "/s/abc123"})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if string(response.Data) != "hello" || response.FileName != "hello.txt" || response.Truncated {
		t.Fatalf("unexpected downloaded asset: %+v", response)
	}
	if metadataRequests != 1 || publicRequests != 2 || objectRequests != 1 {
		t.Fatalf("unexpected request counts: metadata=%d public=%d object=%d", metadataRequests, publicRequests, objectRequests)
	}
}

func TestManagementRedirects(t *testing.T) {
	for _, operation := range []string{"upload", "metadata"} {
		for _, destination := range []string{"same API", "other origin", "public path", "other scheme"} {
			t.Run(operation+"/"+destination, func(t *testing.T) {
				var targetRequests, foreignRequests int
				foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					foreignRequests++
					if r.Header.Get("Authorization") != "" {
						t.Error("cross-origin redirect leaked Authorization")
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				defer foreign.Close()

				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer test-asset-key" {
						t.Error("management request is missing its Bearer API key")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					if r.URL.Path == "/base/api/assets" || r.URL.Path == "/base/api/links/abc123" {
						target := "/base/api/redirected"
						switch destination {
						case "other origin":
							target = foreign.URL + "/base/api/redirected"
						case "public path":
							target = "/base/s/abc123"
						case "other scheme":
							target = "https://" + r.Host + "/base/api/redirected"
						}
						http.Redirect(w, r, target, http.StatusTemporaryRedirect)
						return
					}
					targetRequests++
					if r.URL.Path != "/base/api/redirected" {
						t.Error("management request reached a public path")
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					if operation == "upload" {
						if r.Method != http.MethodPost {
							t.Errorf("redirect changed upload method to %s", r.Method)
						}
						if err := r.ParseMultipartForm(1024); err != nil {
							t.Errorf("redirect lost upload body: %v", err)
						}
						_ = json.NewEncoder(w).Encode(UploadFileResponse{Code: "abc123", ShortURL: serverURL(r) + "/base/s/abc123"})
					} else {
						_ = json.NewEncoder(w).Encode(LinkInfo{Code: "abc123", Kind: "object"})
					}
				}))
				defer server.Close()
				client, err := NewClient(Config{BaseURL: server.URL + "/base", APIKey: "test-asset-key"})
				if err != nil {
					t.Fatalf("new client: %v", err)
				}
				if operation == "upload" {
					_, err = client.UploadFile(context.Background(), UploadFileRequest{Reader: strings.NewReader("hello"), FileName: "hello.txt"})
				} else {
					_, err = client.GetLinkInfo(context.Background(), "abc123")
				}
				if destination == "same API" {
					if err != nil || targetRequests != 1 {
						t.Fatalf("same-origin management redirect failed: requests=%d err=%v", targetRequests, err)
					}
				} else if err == nil || err.Error() != "asset management request failed" || targetRequests != 0 {
					t.Fatalf("unsafe redirect was not blocked: requests=%d err=%v", targetRequests, err)
				}
				if foreignRequests != 0 {
					t.Fatalf("management request reached a foreign origin %d times", foreignRequests)
				}
			})
		}
	}
}

func TestManagementErrorDoesNotEchoCredentials(t *testing.T) {
	for _, operation := range []string{"upload", "metadata"} {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
			t.Run(operation+"/"+http.StatusText(status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "reflected "+r.Header.Get("Authorization"))
				}))
				defer server.Close()
				client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-asset-key"})
				if err != nil {
					t.Fatalf("new client: %v", err)
				}
				if operation == "upload" {
					_, err = client.UploadFile(context.Background(), UploadFileRequest{Reader: strings.NewReader("hello"), FileName: "hello.txt"})
				} else {
					_, err = client.GetLinkInfo(context.Background(), "abc123")
				}
				if err == nil || strings.Contains(err.Error(), "test-asset-key") || strings.Contains(err.Error(), "reflected") {
					t.Fatalf("expected safe management error, got %v", err)
				}
				if status != http.StatusInternalServerError && !strings.Contains(err.Error(), "ASSET_API_KEY") {
					t.Fatalf("expected actionable authentication error, got %v", err)
				}
			})
		}
	}
}

func TestInvalidRedirectLocationDoesNotEchoCredentials(t *testing.T) {
	const apiKey = "fixture-private-asset-key"
	location := "/api/" + apiKey + "/%zz?Authorization=Bearer%20" + apiKey
	for _, operation := range []string{"upload", "metadata", "download"} {
		t.Run(operation, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if operation == "download" && r.URL.Path == "/api/links/abc123" {
					_ = json.NewEncoder(w).Encode(LinkInfo{Code: "abc123", Kind: "object"})
					return
				}
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusTemporaryRedirect)
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: apiKey})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			redirectChecks := 0
			checkRedirect := func(r *http.Request, via []*http.Request) error {
				redirectChecks++
				return errors.New("unexpected redirect callback")
			}
			client.managementClient.CheckRedirect = checkRedirect
			client.httpClient.CheckRedirect = checkRedirect

			err = assetOperationError(client, operation, "")
			assertSafeErrorChain(t, err, apiKey, location, "Location", "Authorization", "%zz")
			if redirectChecks != 0 {
				t.Fatal("fixture must fail Location parsing before CheckRedirect")
			}
			if errors.Unwrap(err) != nil {
				t.Fatal("invalid redirect error retained an untrusted cause")
			}
		})
	}
}

func TestRequestErrorsKeepOnlySafeCategories(t *testing.T) {
	const secret = "fixture-private-transport-key"
	plain := errors.New("Authorization: Bearer " + secret)
	cases := []struct {
		name   string
		cause  error
		target error
	}{
		{name: "transport", cause: plain},
		{name: "canceled", cause: fmt.Errorf("%s: %w", secret, context.Canceled), target: context.Canceled},
		{name: "deadline", cause: fmt.Errorf("%s: %w", secret, context.DeadlineExceeded), target: context.DeadlineExceeded},
		{name: "timeout", cause: fmt.Errorf("%s: %w", secret, os.ErrDeadlineExceeded), target: os.ErrDeadlineExceeded},
		{name: "network timeout", cause: &net.DNSError{Err: secret, Name: "private-transport.example", IsTimeout: true}, target: os.ErrDeadlineExceeded},
	}
	for _, operation := range []string{"upload", "metadata", "download"} {
		for _, tc := range cases {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				client, err := NewClient(Config{BaseURL: "http://short.test", APIKey: "test-asset-key"})
				if err != nil {
					t.Fatalf("new client: %v", err)
				}
				transport := assetTestTransport(func(r *http.Request) (*http.Response, error) {
					if operation == "download" && r.URL.Path == "/api/links/abc123" {
						return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":"abc123","kind":"object"}`))}, nil
					}
					return nil, &url.Error{Op: "fixture", URL: "https://private-transport.example/?key=" + secret, Err: tc.cause}
				})
				client.managementClient.Transport = transport
				client.httpClient.Transport = transport

				err = assetOperationError(client, operation, "http://short.test/s/abc123?token="+secret)
				assertSafeErrorChain(t, err, secret, "private-transport.example", "Authorization", "token=")
				if errors.Is(err, tc.cause) {
					t.Fatal("request error retained the untrusted original cause")
				}
				for _, target := range []error{context.Canceled, context.DeadlineExceeded, os.ErrDeadlineExceeded} {
					if got, want := errors.Is(err, target), target == tc.target; got != want {
						t.Errorf("errors.Is(%v): got %v want %v", target, got, want)
					}
				}
				var timeout interface{ Timeout() bool }
				if tc.target == os.ErrDeadlineExceeded && (!errors.As(err, &timeout) || !timeout.Timeout()) {
					t.Error("safe timeout must retain the timeout category")
				}
			})
		}
	}
}

func TestResponseReadErrorsDoNotEchoCredentials(t *testing.T) {
	const secret = "fixture-private-response-key"
	for _, operation := range []string{"upload", "metadata", "download"} {
		t.Run(operation, func(t *testing.T) {
			client, err := NewClient(Config{BaseURL: "http://short.test", APIKey: "test-asset-key"})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			cause := errors.New("response read failed for Location Authorization " + secret)
			transport := assetTestTransport(func(r *http.Request) (*http.Response, error) {
				var body io.ReadCloser = failingAssetBody{cause}
				if operation == "download" && r.URL.Path == "/api/links/abc123" {
					body = io.NopCloser(strings.NewReader(`{"code":"abc123","kind":"object"}`))
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
			})
			client.managementClient.Transport = transport
			client.httpClient.Transport = transport
			err = assetOperationError(client, operation, "")
			assertSafeErrorChain(t, err, secret, "Location", "Authorization")
			if errors.Unwrap(err) != nil {
				t.Fatal("response read error retained an untrusted cause")
			}
		})
	}
}

func TestPublicDownloadHTTPErrorDoesNotEchoCredentials(t *testing.T) {
	const secret = "fixture-private-signed-url-key"
	var statusCode int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/links/abc123":
			_ = json.NewEncoder(w).Encode(LinkInfo{Code: "abc123", Kind: "object"})
		case "/s/abc123":
			http.Redirect(w, r, "/object?signature="+secret, http.StatusFound)
		case "/object":
			if r.Header.Get("Authorization") != "" {
				t.Error("public object download included Authorization")
			}
			w.WriteHeader(statusCode)
			_, _ = io.WriteString(w, "Location: "+r.URL.String()+"; Authorization: Bearer "+secret)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-asset-key"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			statusCode = status
			err := assetOperationError(client, "download", "")
			assertSafeErrorChain(t, err, secret, "signature=", "Location", "Authorization")
			want := fmt.Sprintf("asset download failed: HTTP %d %s", status, http.StatusText(status))
			if err.Error() != want {
				t.Fatalf("unexpected safe HTTP error: %v", err)
			}
		})
	}
}

func TestPublicURLValidationErrorsDoNotEchoCredentials(t *testing.T) {
	const secret = "fixture-private-source-url-key"
	client, err := NewClient(Config{BaseURL: "http://short.test/base?token=" + secret})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	for _, source := range []string{
		"http://short.test/base/" + secret + "/%zz",
		"http://short.test/outside/" + secret,
		"http://short.test/base/not-a-short-link?token=" + secret,
		"http://user:" + secret + "@short.test/base/s/abc123",
		"http://" + secret + ".other.test/base/s/abc123",
	} {
		_, err := client.DownloadAsset(context.Background(), DownloadAssetRequest{URL: source})
		assertSafeErrorChain(t, err, secret, source, "token=")
	}
}

func assetOperationError(client *Client, operation string, sourceURL string) error {
	switch operation {
	case "upload":
		_, err := client.UploadFile(context.Background(), UploadFileRequest{Reader: strings.NewReader("hello"), FileName: "hello.txt"})
		return err
	case "metadata":
		_, err := client.GetLinkInfo(context.Background(), "abc123")
		return err
	default:
		request := DownloadAssetRequest{URL: sourceURL}
		if sourceURL == "" {
			request.Code = "abc123"
		}
		_, err := client.DownloadAsset(context.Background(), request)
		return err
	}
}

func assertSafeErrorChain(t *testing.T, err error, sensitive ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a safe error")
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			text := fmt.Sprintf(format, current)
			for _, value := range sensitive {
				if strings.Contains(text, value) {
					t.Fatalf("error format %s exposed sensitive fixture data", format)
				}
			}
		}
	}
}

type assetTestTransport func(*http.Request) (*http.Response, error)

func (transport assetTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type failingAssetBody struct{ err error }

func (body failingAssetBody) Read([]byte) (int, error) { return 0, body.err }
func (body failingAssetBody) Close() error             { return nil }

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
