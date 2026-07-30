package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQBAddMagnet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v2/torrents/add" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	client := NewQBClient(server.URL, "", "")
	_, err := client.AddMagnet("magnet:?xt=urn:btih:test", "/tmp/test")
	if err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
}

func TestQBAddTorrentFile(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v2/torrents/add" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		called = true
		w.WriteHeader(200)
	}))
	defer server.Close()

	client := NewQBClient(server.URL, "", "")
	_, err := client.AddTorrentFile([]byte("torrent data here"), "/tmp/test")
	if err != nil {
		t.Fatalf("AddTorrentFile: %v", err)
	}
	if !called {
		t.Error("server was not called")
	}
}

func TestQBList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/info" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"hash":      "abc123",
				"name":      "test",
				"progress":  1.0,
				"state":     "seeding",
				"save_path": "/tmp",
				"total_size": 1024,
			},
		})
	}))
	defer server.Close()

	client := NewQBClient(server.URL, "", "")
	list, err := client.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(list))
	}
	if list[0].Hash != "abc123" {
		t.Errorf("hash: got %q, want abc123", list[0].Hash)
	}
	if list[0].State != "seeding" {
		t.Errorf("state: got %q, want seeding", list[0].State)
	}
	if list[0].Size != 1024 {
		t.Errorf("size: got %d, want 1024", list[0].Size)
	}
}

func TestQBGetInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"hash":      "abc123",
				"name":      "test",
				"progress":  0.5,
				"state":     "downloading",
				"save_path": "/tmp",
				"total_size": 2048,
			},
		})
	}))
	defer server.Close()

	client := NewQBClient(server.URL, "", "")
	info, err := client.GetInfo("abc123")
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.Progress != 0.5 {
		t.Errorf("progress: got %f, want 0.5", info.Progress)
	}
	if info.State != "downloading" {
		t.Errorf("state: got %q, want downloading", info.State)
	}
}

func TestQBDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/delete" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	client := NewQBClient(server.URL, "", "")
	if err := client.Delete("abc123", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestQBTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/app/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte("v4.5.0"))
	}))
	defer server.Close()

	client := NewQBClient(server.URL, "", "")
	if err := client.Test(); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestQBErrorHandling(t *testing.T) {
	// Server returning error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte("Forbidden"))
	}))
	defer server.Close()

	client := NewQBClient(server.URL, "", "")
	_, err := client.AddMagnet("magnet:?xt=urn:btih:test", "/tmp")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestQBMapStates(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"downloading", "downloading"},
		{"metaDL", "downloading"},
		{"stalledDL", "downloading"},
		{"seeding", "seeding"},
		{"uploading", "seeding"},
		{"stalledUP", "seeding"},
		{"pausedDL", "paused"},
		{"pausedUP", "paused"},
		{"error", "error"},
		{"missingFiles", "error"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := mapQBState(tt.input)
		if got != tt.want {
			t.Errorf("mapQBState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
