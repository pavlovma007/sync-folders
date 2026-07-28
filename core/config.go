package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync-folders/db"

	"gopkg.in/yaml.v3"
)

// ConfigPath — путь к служебной директории приложения.
const ConfigPath = ".config/sync-app"

// openDB открывает БД и возвращает Journal.
func openDB() (*db.Journal, error) {
	j, err := db.Open()
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return j, nil
}

// LoadConfig загружает состояние (папки + конфиги) из SQLite.
func LoadConfig() (*Config, error) {
	j, err := openDB()
	if err != nil {
		return nil, err
	}
	defer j.Close()

	cfg := &Config{
		Folders: []Folder{},
		Syncs:   map[string]SyncConfig{},
	}

	// Загружаем папки
	folderRecords, err := j.ListFolders()
	if err != nil {
		return nil, fmt.Errorf("load folders: %w", err)
	}
	for _, fr := range folderRecords {
		cfg.Folders = append(cfg.Folders, Folder{
			Name: fr.Name,
			Path: fr.Path,
		})
	}

	// Загружаем конфиги
	configRecords, err := j.ListSyncConfigs()
	if err != nil {
		return nil, fmt.Errorf("load configs: %w", err)
	}
	for _, cr := range configRecords {
		cfg.Syncs[cr.Name] = recordToSyncConfig(cr)
	}

	return cfg, nil
}

// SaveConfig сохраняет состояние в SQLite.
// Заменяет все существующие данные новыми.
func SaveConfig(cfg *Config) error {
	j, err := openDB()
	if err != nil {
		return err
	}
	defer j.Close()

	// Очищаем старые данные перед вставкой
	if err := j.ClearAllFolders(); err != nil {
		return fmt.Errorf("clear folders: %w", err)
	}
	if err := j.ClearAllSyncConfigs(); err != nil {
		return fmt.Errorf("clear configs: %w", err)
	}

	// Вставляем папки
	for _, f := range cfg.Folders {
		if err := j.AddFolder(f.Name, f.Path); err != nil {
			return fmt.Errorf("save folder %q: %w", f.Name, err)
		}
	}

	// Вставляем конфиги
	for name, sc := range cfg.Syncs {
		rec := syncConfigToRecord(name, sc)
		if err := j.AddSyncConfig(name, rec); err != nil {
			return fmt.Errorf("save config %q: %w", name, err)
		}
	}

	return nil
}

// AddFolder добавляет папку.
func AddFolder(name, path string) error {
	j, err := openDB()
	if err != nil {
		return err
	}
	defer j.Close()
	return j.AddFolder(name, path)
}

// RemoveFolder удаляет папку.
func RemoveFolder(name string) error {
	j, err := openDB()
	if err != nil {
		return err
	}
	defer j.Close()
	return j.RemoveFolder(name)
}

// ListFolders возвращает список папок.
func ListFolders() ([]Folder, error) {
	j, err := openDB()
	if err != nil {
		return nil, err
	}
	defer j.Close()

	records, err := j.ListFolders()
	if err != nil {
		return nil, err
	}

	var result []Folder
	for _, r := range records {
		result = append(result, Folder{Name: r.Name, Path: r.Path})
	}
	return result, nil
}

// AddConfig добавляет конфиг из YAML файла.
func AddConfig(yamlPath string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	var sc SyncConfig
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}
	if sc.Folder == "" {
		return fmt.Errorf("config missing 'folder' field")
	}

	// имя конфига = имя файла без .yaml
	name := filepath.Base(yamlPath)
	name = name[:len(name)-len(filepath.Ext(name))]

	j, err := openDB()
	if err != nil {
		return err
	}
	defer j.Close()

	rec := syncConfigToRecord(name, sc)
	return j.AddSyncConfig(name, rec)
}

// RemoveConfig удаляет конфиг по имени.
func RemoveConfig(name string) error {
	j, err := openDB()
	if err != nil {
		return err
	}
	defer j.Close()
	return j.RemoveSyncConfig(name)
}

// ListConfigs возвращает все конфиги.
func ListConfigs() (map[string]SyncConfig, error) {
	j, err := openDB()
	if err != nil {
		return nil, err
	}
	defer j.Close()

	records, err := j.ListSyncConfigs()
	if err != nil {
		return nil, err
	}

	result := make(map[string]SyncConfig, len(records))
	for _, cr := range records {
		result[cr.Name] = recordToSyncConfig(cr)
	}
	return result, nil
}

// ─── Конвертеры между db.SyncConfigRecord и core.SyncConfig ───

func recordToSyncConfig(cr db.SyncConfigRecord) SyncConfig {
	return SyncConfig{
		Folder:      cr.Folder,
		Description: cr.Description,
		Transport: TransportConfig{
			Type:   cr.TransportType,
			Config: db.DecodeTransportConfig(cr.TransportConfig),
		},
		Sync: SyncSettings{
			Period:        cr.SyncPeriod,
			Direction:     Direction(cr.SyncDirection),
			Conflict:      ConflictMode(cr.SyncConflict),
			SendFilter:    cr.SyncSendFilter,
			ReceiveFilter: cr.SyncRecvFilter,
		},
	}
}

func syncConfigToRecord(name string, sc SyncConfig) db.SyncConfigRecord {
	return db.SyncConfigRecord{
		Name:            name,
		Folder:          sc.Folder,
		Description:     sc.Description,
		TransportType:   sc.Transport.Type,
		TransportConfig: db.EncodeTransportConfig(sc.Transport.Config),
		SyncPeriod:      sc.Sync.Period,
		SyncDirection:   string(sc.Sync.Direction),
		SyncConflict:    string(sc.Sync.Conflict),
		SyncSendFilter:  sc.Sync.SendFilter,
		SyncRecvFilter:  sc.Sync.ReceiveFilter,
	}
}
