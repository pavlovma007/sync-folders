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
)

// testMysqlEntry — для JSON-ответа mock-сервера.
type testMysqlEntry struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

func TestMySQLName(t *testing.T) {
	c, _ := NewMySQLClient(map[string]string{"url": "http://example.com/storage.php"})
	if c.Name() != "mysql" {
		t.Errorf("expected 'mysql', got %q", c.Name())
	}
}

func TestMySQLNewRequiredFields(t *testing.T) {
	_, err := NewMySQLClient(map[string]string{})
	if err == nil {
		t.Error("expected error for empty url")
	}
}

func TestMySQLNewDefaults(t *testing.T) {
	c, _ := NewMySQLClient(map[string]string{"url": "http://example.com/storage.php"})
	if c.group != "default_group" {
		t.Errorf("expected default group 'default_group', got %q", c.group)
	}
}

func TestMySQLList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем что group передан
		if !strings.Contains(r.URL.RawQuery, "group=my-sync") {
			t.Errorf("expected group=my-sync in query, got %s", r.URL.RawQuery)
		}
		files := []testMysqlEntry{
			{Name: "photo.jpg", Group: "my-sync", Size: 1024, ModTime: 1700000000},
			{Name: "doc.pdf", Group: "my-sync", Size: 2048, ModTime: 1700000100},
		}
		json.NewEncoder(w).Encode(files)
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{
		"url":   server.URL,
		"group": "my-sync",
	})

	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
	}
	if !names["photo.jpg"] {
		t.Error("expected photo.jpg in list")
	}
	if !names["doc.pdf"] {
		t.Error("expected doc.pdf in list")
	}
}

func TestMySQLListEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]testMysqlEntry{})
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{"url": server.URL})
	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestMySQLListByGroup(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]testMysqlEntry{
			{Name: "f1.txt", Group: "test-group", Size: 100, ModTime: 1700000000},
		})
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{
		"url":   server.URL,
		"group": "test-group",
	})

	_, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(capturedQuery, "group=test-group") {
		t.Errorf("expected group=test-group in query, got %s", capturedQuery)
	}
}

func TestMySQLPush(t *testing.T) {
	var receivedFile string
	var receivedGroup string
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
		receivedFile = string(data)
		receivedGroup = r.FormValue("group")

		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{
		"url":   server.URL,
		"group": "test-group",
	})

	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(localFile, []byte("mysql push data"), 0644)

	err := client.Push(localFile, "test.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if receivedFile != "mysql push data" {
		t.Errorf("expected 'mysql push data', got %q", receivedFile)
	}
	if receivedGroup != "test-group" {
		t.Errorf("expected group 'test-group', got %q", receivedGroup)
	}
}

func TestMySQLPull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "file_name=test.txt") {
			t.Errorf("expected file_name=test.txt, got %s", r.URL.RawQuery)
		}
		w.Write([]byte("mysql pull content"))
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{"url": server.URL})
	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "downloaded.txt")

	err := client.Pull("test.txt", localFile)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	data, _ := os.ReadFile(localFile)
	if string(data) != "mysql pull content" {
		t.Errorf("expected 'mysql pull content', got %q", string(data))
	}
}

func TestMySQLPushPullCycle(t *testing.T) {
	var storedData []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			file, _, _ := r.FormFile("file")
			storedData, _ = io.ReadAll(file)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		} else {
			if storedData != nil {
				w.Write(storedData)
			} else {
				w.WriteHeader(404)
			}
		}
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{"url": server.URL})

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "cycle.txt")
	original := []byte("push-pull cycle test")
	os.WriteFile(srcFile, original, 0644)

	err := client.Push(srcFile, "cycle.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	destFile := filepath.Join(tmpDir, "result.txt")
	err = client.Pull("cycle.txt", destFile)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	result, _ := os.ReadFile(destFile)
	if string(result) != string(original) {
		t.Errorf("expected %q, got %q", original, result)
	}
}

func TestMySQLDelete(t *testing.T) {
	client, _ := NewMySQLClient(map[string]string{"url": "http://example.com/storage.php"})
	err := client.Delete("somefile.txt")
	if err != ErrNotSupported {
		t.Errorf("expected ErrNotSupported, got %v", err)
	}
}

func TestMySQLAuth(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode([]testMysqlEntry{})
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{
		"url":  server.URL,
		"auth": "user:pass",
	})
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

func TestMySQLGroupIsolation(t *testing.T) {
	var lastGroupQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		lastGroupQuery = r.URL.Query().Get("group")
		if lastGroupQuery == "group-a" {
			json.NewEncoder(w).Encode([]testMysqlEntry{
				{Name: "from_a.txt", Group: "group-a", Size: 100, ModTime: 1700000000},
			})
		} else {
			json.NewEncoder(w).Encode([]testMysqlEntry{})
		}
	}))
	defer server.Close()

	// Тест с группой A
	clientA, _ := NewMySQLClient(map[string]string{
		"url":   server.URL,
		"group": "group-a",
	})
	filesA, _ := clientA.List("")
	if len(filesA) != 1 {
		t.Errorf("expected 1 file in group-a, got %d", len(filesA))
	}

	// Тест с группой B
	clientB, _ := NewMySQLClient(map[string]string{
		"url":   server.URL,
		"group": "group-b",
	})
	filesB, _ := clientB.List("")
	if len(filesB) != 0 {
		t.Errorf("expected 0 files in group-b, got %d", len(filesB))
	}
}

func TestMySQLStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, "internal error")
	}))
	defer server.Close()

	client, _ := NewMySQLClient(map[string]string{"url": server.URL})
	_, err := client.List("")
	if err == nil {
		t.Error("expected error for 500 status")
	}
}
