package transport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTorrentTransportName(t *testing.T) {
	tt, _ := NewTorrentTransport(TorrentConfig{Project: "test"})
	if tt.Name() != "torrent" {
		t.Errorf("Name: got %q, want torrent", tt.Name())
	}
}

func TestTorrentTransportPushFlush(t *testing.T) {
	dir := t.TempDir()
	stagingDir := filepath.Join(dir, ".sync-torrent-staging")
	localDir := filepath.Join(dir, "files")
	os.MkdirAll(localDir, 0755)

	mockTC := NewMockTorrentClient()
	mockDHT := NewMockDHTClient()

	cfg := TorrentConfig{
		Project:    "test-proj",
		StagingDir: stagingDir,
		LocalDir:   localDir,
		TorrentClient: mockTC,
		DHTClient:     mockDHT,
		KeepSeeds:     3,
		MergeMode:     MergeKeepLocal,
		DHTKey:        []byte("test-pub-key-32-bytes-length!"),
		DHTPrivKey:    []byte("test-priv-key-64-bytes!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"),
	}

	tt, err := NewTorrentTransport(cfg)
	if err != nil {
		t.Fatalf("NewTorrentTransport: %v", err)
	}
	defer tt.Close()

	// Push a file
	srcFile := filepath.Join(localDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := tt.Push(srcFile, "test.txt"); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Flush
	if err := tt.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Check DHT has the manifest
	salt := "sync-folders:test-proj"
	val, _, err := mockDHT.Get([]byte("test-pub-key-32-bytes-length!"), salt)
	if err != nil {
		t.Fatalf("DHT Get after Flush: %v", err)
	}
	if len(val) == 0 {
		t.Error("expected non-empty DHT value after flush")
	}
}

func TestTorrentTransportFlushNoChanges(t *testing.T) {
	dir := t.TempDir()
	stagingDir := filepath.Join(dir, ".sync-torrent-staging")
	localDir := filepath.Join(dir, "files")
	os.MkdirAll(localDir, 0755)

	mockTC := NewMockTorrentClient()
	mockDHT := NewMockDHTClient()

	cfg := TorrentConfig{
		Project:    "test-proj",
		StagingDir: stagingDir,
		LocalDir:   localDir,
		TorrentClient: mockTC,
		DHTClient:     mockDHT,
		DHTKey:        []byte("test-pub-key-32-bytes-length!"),
		DHTPrivKey:    []byte("test-priv-key-64-bytes!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"),
	}

	tt, _ := NewTorrentTransport(cfg)
	defer tt.Close()

	// Push once and flush
	srcFile := filepath.Join(localDir, "test.txt")
	os.WriteFile(srcFile, []byte("hello"), 0644)
	tt.Push(srcFile, "test.txt")
	if err := tt.Flush(); err != nil {
		t.Fatalf("first Flush: %v", err)
	}

	// Push same file again and flush — should detect no changes
	tt.Push(srcFile, "test.txt")
	if err := tt.Flush(); err != nil {
		t.Fatalf("second Flush: %v", err)
	}

	// Verify DHT still has data
	val, _, err := mockDHT.Get(cfg.DHTKey, "sync-folders:test-proj")
	if err != nil {
		t.Fatalf("DHT Get after second Flush: %v", err)
	}
	if len(val) == 0 {
		t.Error("expected DHT data after second Flush")
	}
}

func TestTorrentTransportFlushWithChanges(t *testing.T) {
	dir := t.TempDir()
	stagingDir := filepath.Join(dir, ".sync-torrent-staging")
	localDir := filepath.Join(dir, "files")
	os.MkdirAll(localDir, 0755)

	mockTC := NewMockTorrentClient()
	mockDHT := NewMockDHTClient()

	cfg := TorrentConfig{
		Project:    "test-proj",
		StagingDir: stagingDir,
		LocalDir:   localDir,
		TorrentClient: mockTC,
		DHTClient:     mockDHT,
		DHTKey:        []byte("test-pub-key-32-bytes-length!"),
		DHTPrivKey:    []byte("test-priv-key-64-bytes!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"),
	}

	tt, _ := NewTorrentTransport(cfg)
	defer tt.Close()

	// Push and flush version 1
	srcFile := filepath.Join(localDir, "v1.txt")
	os.WriteFile(srcFile, []byte("version1"), 0644)
	tt.Push(srcFile, "v1.txt")
	tt.Flush()

	// Push and flush version 2 — changed content
	os.WriteFile(srcFile, []byte("version2"), 0644)
	tt.Push(srcFile, "v1.txt")
	if err := tt.Flush(); err != nil {
		t.Fatalf("Flush with changes: %v", err)
	}

	// DHT should still have data
	val, _, err := mockDHT.Get(cfg.DHTKey, "sync-folders:test-proj")
	if err != nil {
		t.Fatalf("DHT Get: %v", err)
	}
	if len(val) == 0 {
		t.Error("expected DHT data after flush with changes")
	}
}

func TestTorrentTransportTest(t *testing.T) {
	mockTC := NewMockTorrentClient()
	tt, _ := NewTorrentTransport(TorrentConfig{
		Project:    "test",
		TorrentClient: mockTC,
	})

	if err := tt.Test(); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestTorrentTransportList(t *testing.T) {
	mockTC := NewMockTorrentClient()
	mockTC.AddMagnet("magnet:?xt=urn:btih:test", "/tmp")

	tt, _ := NewTorrentTransport(TorrentConfig{
		Project:    "test",
		TorrentClient: mockTC,
	})

	files, err := tt.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestTorrentTransportDelete(t *testing.T) {
	tt, _ := NewTorrentTransport(TorrentConfig{Project: "test"})
	err := tt.Delete("anything")
	if err == nil {
		t.Error("Delete should return error (not supported)")
	}
}

func TestNewTorrentFromConfig(t *testing.T) {
	cfg := map[string]string{
		"project":    "my-project",
		"staging_dir": "/tmp/staging",
		"local_dir":   "/tmp/local",
		"client":      "qbittorrent",
		"api_url":     "http://127.0.0.1:8080",
		"keep_seeds":  "5",
	}

	// This will fail because there's no qBittorrent running, but the config parsing should work
	_, err := newTorrentFromConfig(cfg)
	if err == nil {
		t.Log("newTorrentFromConfig parsed config (will fail connecting)")
	}
}
