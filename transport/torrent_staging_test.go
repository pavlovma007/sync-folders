package transport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStagingAddFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := s.Add(srcFile, "remote/test.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	stagedPath := filepath.Join(dir, "latest", "remote", "test.txt")
	data, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("staged file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content: got %q, want hello", string(data))
	}
}

func TestStagingAddPreservesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("aaa"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "c.txt"), []byte("ccc"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add a.txt: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "b.txt"), "b.txt"); err != nil {
		t.Fatalf("Add b.txt: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "sub", "c.txt"), "sub/c.txt"); err != nil {
		t.Fatalf("Add sub/c.txt: %v", err)
	}

	for _, tc := range []struct{ path, content string }{
		{filepath.Join(dir, "latest", "a.txt"), "aaa"},
		{filepath.Join(dir, "latest", "b.txt"), "bbb"},
		{filepath.Join(dir, "latest", "sub", "c.txt"), "ccc"},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Errorf("expected %s: %v", tc.path, err)
			continue
		}
		if string(data) != tc.content {
			t.Errorf("%s: got %q, want %q", tc.path, string(data), tc.content)
		}
	}
}

func TestStagingBuildSnapshot(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("file-a"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("file-b"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add a.txt: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "b.txt"), "sub/b.txt"); err != nil {
		t.Fatalf("Add sub/b.txt: %v", err)
	}

	snapshot, manifest, err := s.BuildSnapshot("test-project")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	if len(snapshot.TorrentData) == 0 {
		t.Error("expected non-empty torrent data")
	}
	if snapshot.Magnet == "" {
		t.Error("expected non-empty magnet URI")
	}
	if !strings.HasPrefix(snapshot.Magnet, "magnet:") {
		t.Errorf("magnet should start with 'magnet:', got %q", snapshot.Magnet)
	}
	if len(manifest.Files) == 0 {
		t.Error("expected files in manifest")
	}
	if len(manifest.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(manifest.Files))
	}
}

func TestStagingDiffNoChanges(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	_, manifest, err := s.BuildSnapshot("test")
	if err != nil {
		t.Fatalf("first BuildSnapshot: %v", err)
	}

	if err := s.SaveLastManifest(manifest); err != nil {
		t.Fatalf("SaveLastManifest: %v", err)
	}

	// Публикуем тот же файл снова
	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	hasChanges, err := s.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if hasChanges {
		t.Error("expected no changes, but HasChanges returned true")
	}
}

func TestStagingDiffHasChanges(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()

	// First version
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, manifest, err := s.BuildSnapshot("test")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if err := s.SaveLastManifest(manifest); err != nil {
		t.Fatalf("SaveLastManifest: %v", err)
	}

	// Second version — changed content
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("changed!"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	hasChanges, err := s.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if !hasChanges {
		t.Error("expected changes, but HasChanges returned false")
	}
}

func TestStagingDiffNewFile(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()

	// First version — one file
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, manifest, err := s.BuildSnapshot("test")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if err := s.SaveLastManifest(manifest); err != nil {
		t.Fatalf("SaveLastManifest: %v", err)
	}

	// Second version — add b.txt
	if err := os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("new file"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "a.txt"), "a.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "b.txt"), "b.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	hasChanges, err := s.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges: %v", err)
	}
	if !hasChanges {
		t.Error("expected changes (new file), but HasChanges returned false")
	}
}

func TestStagingSaveAndReadManifest(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "f1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "f2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s.Add(filepath.Join(srcDir, "f1.txt"), "f1.txt")
	s.Add(filepath.Join(srcDir, "f2.txt"), "sub/f2.txt")

	_, manifest, err := s.BuildSnapshot("test")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	if err := s.SaveLastManifest(manifest); err != nil {
		t.Fatalf("SaveLastManifest: %v", err)
	}

	// Verify manifest file exists and is readable
	manifestPath := filepath.Join(dir, ".torrent-last-manifest.ndjson")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	if len(data) == 0 {
		t.Error("manifest file is empty")
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestStagingClear(t *testing.T) {
	dir := t.TempDir()
	s := NewStaging(dir)

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := s.Add(filepath.Join(srcDir, "f.txt"), "f.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Verify staging exists
	if _, err := os.Stat(filepath.Join(dir, "latest", "f.txt")); os.IsNotExist(err) {
		t.Error("staging file should exist before Clear")
	}

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	// Verify staging is gone
	if _, err := os.Stat(filepath.Join(dir, "latest")); !os.IsNotExist(err) {
		t.Error("staging should be empty after Clear")
	}
}
