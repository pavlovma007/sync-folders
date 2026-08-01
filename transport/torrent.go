package transport

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"sync-folders/dht"
)

// MergeMode определяет стратегию слияния при Pull.
type MergeMode string

const (
	MergeKeepLocal MergeMode = "keep_local"
	MergeMirror    MergeMode = "mirror_remote"
)

// TorrentConfig — конфигурация TorrentTransport.
type TorrentConfig struct {
	Project    string
	StagingDir string // .sync-torrent-staging/ path
	LocalDir   string // целевая локальная папка

	TorrentClient TorrentClient
	DHTClient     dht.DHTClient

	DHTKey     []byte // Ed25519 публичный ключ
	DHTPrivKey []byte // Ed25519 приватный ключ

	KeepSeeds  int
	MaxSeedAge time.Duration
	MergeMode  MergeMode

	PollInterval time.Duration // для Pull-цикла (default 30s)
}

// TorrentTransport — Transport интерфейс для торрент-синхронизации.
type TorrentTransport struct {
	cfg     TorrentConfig
	staging *Staging

	mu      sync.Mutex
	lastSeq int64 // последний опубликованный seq

	// Для Pull-цикла
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTorrentTransport создаёт TorrentTransport.
func NewTorrentTransport(cfg TorrentConfig) (*TorrentTransport, error) {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.KeepSeeds == 0 {
		cfg.KeepSeeds = 3
	}
	if cfg.MergeMode == "" {
		cfg.MergeMode = MergeKeepLocal
	}

	tt := &TorrentTransport{
		cfg:     cfg,
		staging: NewStaging(cfg.StagingDir),
	}

	// Start pull cycle (только если настроены DHT-ключи)
	if len(cfg.DHTKey) > 0 {
		tt.ctx, tt.cancel = context.WithCancel(context.Background())
		go tt.pullCycle()
	}

	return tt, nil
}

func (tt *TorrentTransport) Name() string { return "torrent" }

// Push копирует файл в staging для последующего snapshot.
func (tt *TorrentTransport) Push(localPath, remotePath string) error {
	return tt.staging.Add(localPath, remotePath)
}

// Flush проверяет изменения и публикует новый снапшот если нужно.
func (tt *TorrentTransport) Flush() error {
	tt.mu.Lock()
	defer tt.mu.Unlock()

	// 1. Проверить изменения
	hasChanges, err := tt.staging.HasChanges()
	if err != nil {
		return fmt.Errorf("flush check changes: %w", err)
	}
	if !hasChanges {
		log.Printf("[torrent] no changes since last snapshot, skipping")
		return nil
	}

	// 2. Создать снапшот
	snapshot, stagingManifest, err := tt.staging.BuildSnapshot(tt.cfg.Project)
	if err != nil {
		return fmt.Errorf("flush build snapshot: %w", err)
	}
	log.Printf("[torrent] snapshot built: %s (%d files)", snapshot.Magnet, len(stagingManifest.Files))

	// 3. Скопировать staging/latest/ в seed-директорию и добавить .torrent в qBittorrent
	if tt.cfg.TorrentClient != nil {
		seedDir, err := tt.staging.PromoteToSeed(tt.cfg.Project)
		if err != nil {
			return fmt.Errorf("flush promote to seed: %w", err)
		}
		_, err = tt.cfg.TorrentClient.AddTorrentFile(snapshot.TorrentData, seedDir)
		if err != nil {
			return fmt.Errorf("flush add torrent: %w", err)
		}
		log.Printf("[torrent] added to client for seeding (savepath=%s)", seedDir)
	}

	// 4. Опубликовать манифест в DHT
	if tt.cfg.DHTClient != nil && len(tt.cfg.DHTKey) > 0 {
		seq := stagingManifest.Seq
		dhtManifest := dht.Manifest{
			Seq:       seq,
			Magnet:    snapshot.Magnet,
			Timestamp: stagingManifest.Timestamp,
			FilesHash: stagingManifest.FilesHash,
		}
		value, err := dhtManifest.Marshal()
		if err != nil {
			return fmt.Errorf("flush marshal manifest: %w", err)
		}

		err = tt.cfg.DHTClient.Put(tt.cfg.DHTKey, tt.cfg.DHTPrivKey,
			dht.SaltForProject(tt.cfg.Project), seq, value)
		if err != nil {
			return fmt.Errorf("flush dht put: %w", err)
		}
		log.Printf("[torrent] DHT published seq=%d", seq)
	}

	// 5. Сохранить манифест для будущих diff
	if err := tt.staging.SaveLastManifest(stagingManifest); err != nil {
		return fmt.Errorf("flush save manifest: %w", err)
	}

	// 6. Очистить staging
	if err := tt.staging.Clear(); err != nil {
		return fmt.Errorf("flush clear staging: %w", err)
	}

	tt.lastSeq = stagingManifest.Seq
	return nil
}

// Pull проверяет наличие файла локально (приходит через фоновый pull-cycle).
func (tt *TorrentTransport) Pull(remotePath, localPath string) error {
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return fmt.Errorf("torrent pull: file not yet downloaded (pull cycle in progress)")
	}
	return nil
}

// List возвращает список файлов от торрент-клиента.
func (tt *TorrentTransport) List(remotePath string) ([]FileInfo, error) {
	if tt.cfg.TorrentClient == nil {
		return nil, fmt.Errorf("torrent: torrent client not configured")
	}
	torrents, err := tt.cfg.TorrentClient.List()
	if err != nil {
		return nil, fmt.Errorf("torrent list: %w", err)
	}
	result := make([]FileInfo, 0, len(torrents))
	for _, t := range torrents {
		result = append(result, FileInfo{
			Name: t.Name,
			Path: t.Hash,
			Size: t.Size,
		})
	}
	return result, nil
}

// Delete — не поддерживается напрямую (управляется через keep_seeds).
func (tt *TorrentTransport) Delete(remotePath string) error {
	return fmt.Errorf("torrent: delete not supported, use keep_seeds / max_seed_age")
}

// Test проверяет DHT и торрент-клиент.
func (tt *TorrentTransport) Test() error {
	if tt.cfg.TorrentClient != nil {
		if err := tt.cfg.TorrentClient.Test(); err != nil {
			return fmt.Errorf("torrent client: %w", err)
		}
	}
	return nil
}

// Close останавливает Pull-цикл.
func (tt *TorrentTransport) Close() error {
	if tt.cancel != nil {
		tt.cancel()
	}
	if tt.cfg.DHTClient != nil {
		return tt.cfg.DHTClient.Close()
	}
	return nil
}

// pullCycle — фоновая горутина для обнаружения новых версий.
func (tt *TorrentTransport) pullCycle() {
	ticker := time.NewTicker(tt.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tt.ctx.Done():
			return
		case <-ticker.C:
			tt.checkForUpdates()
		}
	}
}

func (tt *TorrentTransport) checkForUpdates() {
	pub := tt.cfg.DHTKey
	salt := dht.SaltForProject(tt.cfg.Project)

	value, seq, err := tt.cfg.DHTClient.Get(pub, salt)
	if err != nil {
		log.Printf("[torrent] pull check: DHT get error: %v", err)
		return
	}
	if seq <= tt.lastSeq {
		return
	}

	dm, err := dht.UnmarshalManifest(value)
	if err != nil {
		log.Printf("[torrent] pull check: parse manifest: %v", err)
		return
	}

	log.Printf("[torrent] pull: new version seq=%d, magnet=%s", seq, dm.Magnet)

	if tt.cfg.TorrentClient == nil {
		log.Printf("[torrent] pull: no torrent client configured, skipping download")
		return
	}

	// Добавляем magnet в торрент-клиент
	hash, err := tt.cfg.TorrentClient.AddMagnet(dm.Magnet, tt.cfg.LocalDir+"/.torrent-downloads")
	if err != nil {
		log.Printf("[torrent] pull: add magnet: %v", err)
		return
	}

	// Ждём завершения загрузки
	if err := tt.waitForDownload(hash); err != nil {
		log.Printf("[torrent] pull: download: %v", err)
		return
	}

	// Сливаем скачанные файлы
	if err := tt.mergeDownloadedFiles(hash); err != nil {
		log.Printf("[torrent] pull: merge: %v", err)
		return
	}

	tt.lastSeq = seq
	log.Printf("[torrent] pull: updated to seq=%d", seq)
}

func (tt *TorrentTransport) waitForDownload(hash string) error {
	timeout := time.After(30 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("download timeout for %s", hash)
		case <-tt.ctx.Done():
			return fmt.Errorf("pull cycle stopped during download")
		case <-ticker.C:
			info, err := tt.cfg.TorrentClient.GetInfo(hash)
			if err != nil {
				return fmt.Errorf("get status: %w", err)
			}
			if info.State == "error" {
				return fmt.Errorf("download error for %s", hash)
			}
			if info.Progress >= 1.0 {
				return nil
			}
		}
	}
}

func (tt *TorrentTransport) mergeDownloadedFiles(hash string) error {
	info, err := tt.cfg.TorrentClient.GetInfo(hash)
	if err != nil {
		return err
	}

	downloadPath := info.SavePath
	if info.Name != "" {
		downloadPath = filepath.Join(downloadPath, info.Name)
	}

	return filepath.Walk(downloadPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, err := filepath.Rel(downloadPath, path)
		if err != nil {
			return nil
		}
		localDest := filepath.Join(tt.cfg.LocalDir, rel)

		// Только копировать новые/изменённые файлы
		if existing, err := os.Stat(localDest); err == nil {
			if existing.Size() == fi.Size() {
				same, _ := filesEqual(path, localDest)
				if same {
					return nil
				}
			}
		}

		return copyFile(path, localDest)
	})
}

func filesEqual(a, b string) (bool, error) {
	ha, err := fileSHA256(a)
	if err != nil {
		return false, err
	}
	hb, err := fileSHA256(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

// newTorrentFromConfig создаёт TorrentTransport из map-конфига.
func newTorrentFromConfig(cfg map[string]string) (*TorrentTransport, error) {
	tc := TorrentConfig{
		Project:    cfg["project"],
		StagingDir: cfg["staging_dir"],
		LocalDir:   cfg["local_dir"],
	}

	// Parse keep_seeds
	if v := cfg["keep_seeds"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			tc.KeepSeeds = n
		}
	}

	// Parse max_seed_age
	if v := cfg["max_seed_age"]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			tc.MaxSeedAge = d
		}
	}

	// Client type
	clientType := cfg["client"]
	if clientType == "" {
		clientType = "qbittorrent"
	}

	switch clientType {
	case "qbittorrent":
		tc.TorrentClient = NewQBClient(
			cfg["api_url"],
			cfg["api_user"],
			cfg["api_password"],
		)
	default:
		return nil, fmt.Errorf("torrent: unknown client type: %s", clientType)
	}

	// Parse DHT keys (hex-encoded)
	if pk := cfg["dht_public_key"]; pk != "" {
		decoded, err := hex.DecodeString(pk)
		if err == nil {
			tc.DHTKey = decoded
		} else {
			tc.DHTKey = []byte(pk)
		}
	}
	if priv := cfg["dht_private_key"]; priv != "" {
		decoded, err := hex.DecodeString(priv)
		if err == nil {
			tc.DHTPrivKey = decoded
		} else {
			tc.DHTPrivKey = []byte(priv)
		}
	}

	// Create DHT client if keys are provided
	if len(tc.DHTKey) > 0 {
		dhtClient, err := dht.NewClient()
		if err != nil {
			return nil, fmt.Errorf("torrent: create dht client: %w", err)
		}
		tc.DHTClient = dhtClient
	}

	return NewTorrentTransport(tc)
}
