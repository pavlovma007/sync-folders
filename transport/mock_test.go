package transport

import (
	"os"
	"path/filepath"
	"testing"
)

func setupMock(t *testing.T) (*Mock, string, string) {
	t.Helper()
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	os.WriteFile(filepath.Join(localDir, "test.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(localDir, "photo.jpg"), []byte("image"), 0644)

	mock, err := NewMock(map[string]string{"root": remoteDir})
	if err != nil {
		t.Fatalf("NewMock: %v", err)
	}
	return mock, localDir, remoteDir
}

func TestMockList(t *testing.T) {
	mock, _, _ := setupMock(t)
	files, err := mock.List("")
	if err != nil {
		t.Fatal(err)
	}
	// remote dir is empty initially
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestMockPush(t *testing.T) {
	mock, localDir, remoteDir := setupMock(t)

	err := mock.Push(filepath.Join(localDir, "test.txt"), "test.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	// check file exists in remote
	data, err := os.ReadFile(filepath.Join(remoteDir, "test.txt"))
	if err != nil {
		t.Fatalf("remote file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func TestMockPushAndList(t *testing.T) {
	mock, localDir, remoteDir := setupMock(t)

	mock.Push(filepath.Join(localDir, "test.txt"), "test.txt")
	mock.Push(filepath.Join(localDir, "photo.jpg"), "sub/photo.jpg")

	files, err := mock.List("")
	if err != nil {
		t.Fatal(err)
	}
	// Считаем только файлы (не директории)
	fileCount := 0
	for _, f := range files {
		if !f.IsDir {
			fileCount++
		}
	}
	if fileCount != 2 {
		t.Errorf("expected 2 files (no dirs), got %d", fileCount)
	}
	_ = remoteDir
}

func TestMockPull(t *testing.T) {
	mock, localDir, remoteDir := setupMock(t)

	// push
	mock.Push(filepath.Join(localDir, "test.txt"), "test.txt")

	// pull to a different location
	tmpDir := t.TempDir()
	err := mock.Pull("test.txt", filepath.Join(tmpDir, "result.txt"))
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "result.txt"))
	if err != nil {
		t.Fatalf("pulled file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
	_ = remoteDir
}

func TestMockDelete(t *testing.T) {
	mock, localDir, remoteDir := setupMock(t)
	mock.Push(filepath.Join(localDir, "test.txt"), "test.txt")

	err := mock.Delete("test.txt")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	files, _ := mock.List("")
	if len(files) != 0 {
		t.Errorf("expected 0 files after delete, got %d", len(files))
	}
	_ = remoteDir
}

func TestMockTest(t *testing.T) {
	mock, _, _ := setupMock(t)
	if err := mock.Test(); err != nil {
		t.Fatalf("Test: %v", err)
	}
}
