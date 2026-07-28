package core

import (
	"os"
	"path/filepath"
	"sync-folders/transport"
	"testing"
)

// setupTest создаёт тестовое окружение: локальную и удалённую (mock) папки.
func setupTest(t *testing.T) (string, string, *transport.Mock) {
	t.Helper()
	localDir := t.TempDir()
	remoteDir := t.TempDir()

	// создать mock транспорт
	mock, err := transport.NewMock(map[string]string{"root": remoteDir})
	if err != nil {
		t.Fatalf("NewMock: %v", err)
	}
	return localDir, remoteDir, mock
}

func TestSyncNewFiles(t *testing.T) {
	localDir, remoteDir, mock := setupTest(t)
	os.WriteFile(filepath.Join(localDir, "hello.txt"), []byte("world"), 0644)

	// регистрируем папку
	AddFolder("test", localDir)

	// создаём конфиг
	cfg := SyncConfig{
		Folder: "test",
		Transport: TransportConfig{
			Type:   "mock",
			Config: map[string]string{"root": remoteDir},
		},
		Sync: SyncSettings{
			Direction: DirectionPush,
		},
	}

	// запускаем синхронизацию
	engine := &SyncEngine{
		config:    cfg,
		localPath: localDir,
		transp:    mock,
	}
	if err := engine.push(); err != nil {
		t.Fatalf("push: %v", err)
	}

	// проверяем что файл появился в remote
	remoteFiles, _ := mock.List("")
	if len(remoteFiles) < 1 {
		t.Error("expected files in remote after push")
	}
}

func TestSyncDirectionPull(t *testing.T) {
	localDir, remoteDir, mock := setupTest(t)
	AddFolder("test-pull", localDir)

	// создаём файл в удалённой папке (имитируем pull)
	os.WriteFile(filepath.Join(remoteDir, "remote.txt"), []byte("from remote"), 0644)

	engine := &SyncEngine{
		config: SyncConfig{
			Folder: "test-pull",
			Sync:   SyncSettings{Direction: DirectionPull},
		},
		localPath: localDir,
		transp:    mock,
	}

	// очищаем список конфигов
	cfg, _ := LoadConfig()
	cfg.Syncs = map[string]SyncConfig{}
	SaveConfig(cfg)

	if err := engine.pull(); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// проверяем что файл появился локально
	if _, err := os.Stat(filepath.Join(localDir, "remote.txt")); os.IsNotExist(err) {
		t.Error("expected remote.txt to be pulled locally")
	}
}

func TestSyncConflictNewer(t *testing.T) {
	localDir, remoteDir, mock := setupTest(t)
	AddFolder("test-conflict", localDir)

	// одинаковый файл в обоих папках, локальный новее
	os.WriteFile(filepath.Join(localDir, "conflict.txt"), []byte("local"), 0644)
	os.WriteFile(filepath.Join(remoteDir, "conflict.txt"), []byte("remote"), 0644)

	engine := &SyncEngine{
		localPath: localDir,
		transp:    mock,
		config: SyncConfig{
			Folder: "test-conflict",
			Sync:   SyncSettings{Direction: DirectionBidirectional, Conflict: ConflictNewerWins},
		},
	}

	// push должен перезаписать удалённый файл локальным
	if err := engine.push(); err != nil {
		t.Fatalf("push: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(remoteDir, "conflict.txt"))
	if string(data) != "local" {
		t.Errorf("expected remote file to be overwritten with 'local', got %q", string(data))
	}
}

func TestGetStatusNoSync(t *testing.T) {
	localDir := t.TempDir()
	AddFolder("status-test", localDir)

	// удаляем старый конфиг если был
	RemoveConfig("test-status")
	cfg := SyncConfig{
		Folder: "status-test",
		Transport: TransportConfig{
			Type:   "mock",
			Config: map[string]string{"root": t.TempDir()},
		},
		Sync: SyncSettings{Direction: DirectionPush},
	}
	SaveConfig(&Config{
		Folders: []Folder{{Name: "status-test", Path: localDir}},
		Syncs:   map[string]SyncConfig{"test-status": cfg},
	})

	si, err := GetStatus("test-status")
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if si.Name != "test-status" {
		t.Errorf("expected name 'test-status', got %q", si.Name)
	}
	if si.Transport != "mock" {
		t.Errorf("expected transport 'mock', got %q", si.Transport)
	}
	if si.Direction != DirectionPush {
		t.Errorf("expected direction 'push', got %q", si.Direction)
	}
	if !si.LastSync.IsZero() {
		t.Error("expected zero LastSync (never synced)")
	}
	if si.LastError != "" {
		t.Errorf("expected no error, got %q", si.LastError)
	}
}

func TestGetStatusAfterSync(t *testing.T) {
	localDir := t.TempDir()
	remoteDir := t.TempDir()
	AddFolder("status-sync", localDir)

	RemoveConfig("test-status-sync")
	cfg := SyncConfig{
		Folder: "status-sync",
		Transport: TransportConfig{
			Type:   "mock",
			Config: map[string]string{"root": remoteDir},
		},
		Sync: SyncSettings{Direction: DirectionPush},
	}
	SaveConfig(&Config{
		Folders: []Folder{{Name: "status-sync", Path: localDir}},
		Syncs:   map[string]SyncConfig{"test-status-sync": cfg},
	})

	// создаём файл и синхронизируем (чтобы появилась запись в журнале)
	os.WriteFile(filepath.Join(localDir, "synced.txt"), []byte("data"), 0644)

	mock, _ := transport.NewMock(map[string]string{"root": remoteDir})
	engine := &SyncEngine{
		name:      "test-status-sync",
		config:    cfg,
		localPath: localDir,
		transp:    mock,
	}
	if err := engine.push(); err != nil {
		t.Fatalf("push: %v", err)
	}

	// проверяем статус после синхронизации
	si, err := GetStatus("test-status-sync")
	if err != nil {
		t.Fatalf("GetStatus after sync: %v", err)
	}

	if si.LastSync.IsZero() {
		t.Error("expected non-zero LastSync after sync")
	}
}

func TestGetStatusNotFound(t *testing.T) {
	_, err := GetStatus("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestGetAllStatusesEmpty(t *testing.T) {
	SaveConfig(&Config{
		Folders: []Folder{},
		Syncs:   map[string]SyncConfig{},
	})

	statuses, err := GetAllStatuses()
	if err != nil {
		t.Fatalf("GetAllStatuses: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(statuses))
	}
}

func TestGetAllStatusesMultiple(t *testing.T) {
	localDirA := t.TempDir()
	localDirB := t.TempDir()
	remoteDir := t.TempDir()

	SaveConfig(&Config{
		Folders: []Folder{
			{Name: "folder-a", Path: localDirA},
			{Name: "folder-b", Path: localDirB},
		},
		Syncs: map[string]SyncConfig{
			"config-a": {
				Folder: "folder-a",
				Transport: TransportConfig{Type: "mock", Config: map[string]string{"root": remoteDir}},
				Sync:    SyncSettings{Direction: DirectionPush},
			},
			"config-b": {
				Folder: "folder-b",
				Transport: TransportConfig{Type: "mock", Config: map[string]string{"root": remoteDir}},
				Sync:    SyncSettings{Direction: DirectionPull},
			},
		},
	})

	statuses, err := GetAllStatuses()
	if err != nil {
		t.Fatalf("GetAllStatuses: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	// проверяем имена
	names := make(map[string]bool)
	for _, s := range statuses {
		names[s.Name] = true
	}
	if !names["config-a"] || !names["config-b"] {
		t.Errorf("expected both configs, got %v", names)
	}
}
