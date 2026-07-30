package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDelugeAddMagnet(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Respond to auth.login
		var req struct {
			Method string        `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method == "auth.login" {
			json.NewEncoder(w).Encode(map[string]any{"result": true, "error": nil, "id": 1})
		} else if req.Method == "core.add_torrent_url" {
			json.NewEncoder(w).Encode(map[string]any{"result": true, "error": nil, "id": 2})
		} else {
			t.Errorf("unexpected method: %s", req.Method)
		}
	}))
	defer server.Close()

	client := NewDelugeClient(server.URL, "testpass")
	_, err := client.AddMagnet("magnet:?xt=urn:btih:test", "/tmp")
	if err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
	if calls < 1 {
		t.Error("expected at least 1 RPC call")
	}
}

func TestDelugeList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method == "auth.login" {
			json.NewEncoder(w).Encode(map[string]any{"result": true, "error": nil, "id": 1})
		} else if req.Method == "core.get_torrents_status" {
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"abc123": map[string]any{
						"hash":              "abc123",
						"name":              "test",
						"progress":          1.0,
						"state":             "Seeding",
						"download_location": "/tmp",
						"total_size":        1024,
					},
				},
				"error": nil,
				"id":    2,
			})
		}
	}))
	defer server.Close()

	client := NewDelugeClient(server.URL, "pass")
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
}

func TestDelugeDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method == "auth.login" {
			json.NewEncoder(w).Encode(map[string]any{"result": true, "error": nil, "id": 1})
		} else if req.Method == "core.remove_torrent" {
			json.NewEncoder(w).Encode(map[string]any{"result": true, "error": nil, "id": 2})
		}
	}))
	defer server.Close()

	client := NewDelugeClient(server.URL, "pass")
	if err := client.Delete("abc123", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDelugeTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method == "auth.login" {
			json.NewEncoder(w).Encode(map[string]any{"result": true, "error": nil, "id": 1})
		} else if req.Method == "daemon.get_method_list" {
			json.NewEncoder(w).Encode(map[string]any{
				"result": []string{"method1", "method2"},
				"error":  nil,
				"id":     2,
			})
		}
	}))
	defer server.Close()

	client := NewDelugeClient(server.URL, "pass")
	if err := client.Test(); err != nil {
		t.Fatalf("Test: %v", err)
	}
}
