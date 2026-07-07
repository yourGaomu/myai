package asset

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadFile(t *testing.T) {
	var gotTitle string
	var gotTTL string
	var gotFileName string
	var gotContent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
