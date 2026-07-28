package transport

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testFileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

func TestHTTPList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		files := []testFileEntry{
			{Name: "photo.jpg.a1b2", Size: 1024, ModTime: 1700000000},
			{Name: "doc.pdf.e5f6", Size: 2048, ModTime: 1700000100},
		}
		json.NewEncoder(w).Encode(files)
	}))
	defer server.Close()

	client, err := NewHTTPClient(map[string]string{"url": server.URL, "base_url": server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestHTTPListEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]testFileEntry{})
	}))
	defer server.Close()

	client, _ := NewHTTPClient(map[string]string{"url": server.URL})
	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestHTTPPush(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", ct)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile error: %v", err)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		received = string(data)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client, _ := NewHTTPClient(map[string]string{"url": server.URL})
	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(localFile, []byte("hello world"), 0644)

	err := client.Push(localFile, "test.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if received != "hello world" {
		t.Errorf("expected 'hello world', got %q", received)
	}
}

func TestHTTPPull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("file content"))
	}))
	defer server.Close()

	client, _ := NewHTTPClient(map[string]string{"url": server.URL, "base_url": server.URL})
	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "downloaded.txt")

	err := client.Pull("test.txt.a1b2", localFile)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	data, _ := os.ReadFile(localFile)
	if string(data) != "file content" {
		t.Errorf("expected 'file content', got %q", string(data))
	}
}

func TestHTTPDelete(t *testing.T) {
	client, _ := NewHTTPClient(map[string]string{"url": "http://example.com/storage.php"})
	err := client.Delete("f.txt")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}
}

func TestHTTPAuth(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode([]testFileEntry{})
	}))
	defer server.Close()

	client, _ := NewHTTPClient(map[string]string{"url": server.URL, "auth": "user:pass"})
	_, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if authHeader == "" {
		t.Error("expected Authorization header")
	}
	if !strings.HasPrefix(authHeader, "Basic ") {
		t.Errorf("expected Basic auth, got %s", authHeader)
	}
}

func TestHTTPAuthFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		json.NewEncoder(w).Encode([]testFileEntry{})
	}))
	defer server.Close()

	clientNoAuth, _ := NewHTTPClient(map[string]string{"url": server.URL})
	_, err := clientNoAuth.List("")
	if err == nil {
		t.Error("expected error for unauthenticated request")
	}

	clientAuth, _ := NewHTTPClient(map[string]string{"url": server.URL, "auth": "user:pass"})
	_, err = clientAuth.List("")
	if err != nil {
		t.Fatalf("List with auth: %v", err)
	}
}

func TestHTTPName(t *testing.T) {
	client, _ := NewHTTPClient(map[string]string{"url": "http://example.com/storage.php"})
	if client.Name() != "http" {
		t.Errorf("expected 'http', got %q", client.Name())
	}
}

func TestHTTPNewRequiredFields(t *testing.T) {
	_, err := NewHTTPClient(map[string]string{})
	if err == nil {
		t.Error("expected error for empty url")
	}
}

func TestHTTPNewAllFields(t *testing.T) {
	client, err := NewHTTPClient(map[string]string{
		"url":               "https://myserver.com/storage.php",
		"base_url":          "https://myserver.com",
		"auth":              "user:pass",
		"self_signed_certs": "true",
	})
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	if client.url != "https://myserver.com/storage.php" {
		t.Errorf("unexpected url: %s", client.url)
	}
	if client.baseURL != "https://myserver.com" {
		t.Errorf("unexpected base_url: %s", client.baseURL)
	}
	if client.auth != "user:pass" {
		t.Errorf("unexpected auth: %s", client.auth)
	}
}

func TestHTTPDefaultBaseURL(t *testing.T) {
	client, _ := NewHTTPClient(map[string]string{"url": "https://myserver.com/subdir/storage.php"})
	expected := "https://myserver.com/subdir"
	if client.baseURL != expected {
		t.Errorf("expected base_url %q, got %q", expected, client.baseURL)
	}
}

func TestHTTPModTime(t *testing.T) {
	now := time.Now().Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]testFileEntry{{Name: "f.txt.abc", Size: 100, ModTime: now}})
	}))
	defer server.Close()

	client, _ := NewHTTPClient(map[string]string{"url": server.URL})
	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].ModTime.Unix() != now {
		t.Errorf("expected mod_time %d, got %d", now, files[0].ModTime.Unix())
	}
}

func TestHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, "internal error")
	}))
	defer server.Close()

	client, _ := NewHTTPClient(map[string]string{"url": server.URL})
	_, err := client.List("")
	if err == nil {
		t.Error("expected error for 500")
	}
}

func TestHTTPInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	client, _ := NewHTTPClient(map[string]string{"url": server.URL})
	_, err := client.List("")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
