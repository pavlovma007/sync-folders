package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransmissionAddMagnet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First request gets 409 with session-id
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "test-session-123")
			w.WriteHeader(409)
			return
		}

		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "torrent-add" {
			t.Errorf("unexpected method: %s", req.Method)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"result": "success",
			"arguments": map[string]any{
				"torrent-added": map[string]any{
					"hashString": "abc123",
					"id":         1,
					"name":       "test",
				},
			},
		})
	}))
	defer server.Close()

	client := NewTransmissionClient(server.URL, "", "")
	hash, err := client.AddMagnet("magnet:?xt=urn:btih:test", "/tmp")
	if err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
	if hash != "abc123" {
		t.Errorf("hash: got %q, want abc123", hash)
	}
}

func TestTransmissionList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "sess")
			w.WriteHeader(409)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"result": "success",
			"arguments": map[string]any{
				"torrents": []map[string]any{
					{
						"id":           1,
						"hashString":   "abc123",
						"name":         "test.torrent",
						"percentDone":  1.0,
						"status":       6,
						"downloadDir":  "/tmp",
						"totalSize":    2048,
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewTransmissionClient(server.URL, "", "")
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
	if list[0].Size != 2048 {
		t.Errorf("size: got %d, want 2048", list[0].Size)
	}
}

func TestTransmissionGetInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "sess")
			w.WriteHeader(409)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"result": "success",
			"arguments": map[string]any{
				"torrents": []map[string]any{
					{
						"id":          1,
						"hashString":  "abc123",
						"name":        "test",
						"percentDone": 0.5,
						"status":      4,
						"downloadDir": "/tmp",
						"totalSize":   4096,
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewTransmissionClient(server.URL, "", "")
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

func TestTransmissionDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "sess")
			w.WriteHeader(409)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"result":    "success",
			"arguments": map[string]any{},
		})
	}))
	defer server.Close()

	client := NewTransmissionClient(server.URL, "", "")
	if err := client.Delete("abc123", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestTransmissionTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			w.Header().Set("X-Transmission-Session-Id", "sess")
			w.WriteHeader(409)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"result": "success",
			"arguments": map[string]any{
				"version": "4.0.0",
			},
		})
	}))
	defer server.Close()

	client := NewTransmissionClient(server.URL, "", "")
	if err := client.Test(); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestTransmissionMapStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{0, "paused"},
		{1, "downloading"},
		{2, "downloading"},
		{3, "downloading"},
		{4, "downloading"},
		{5, "seeding"},
		{6, "seeding"},
		{99, "error"},
	}

	for _, tt := range tests {
		got := mapTransmissionStatus(tt.status)
		if got != tt.want {
			t.Errorf("mapTransmissionStatus(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
