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
