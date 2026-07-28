package db

import (
	"encoding/json"
	"fmt"
)

// FolderRecord представляет папку в БД.
type FolderRecord struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// SyncConfigRecord представляет конфиг синхронизации в БД.
type SyncConfigRecord struct {
	Name            string `json:"name"`
	Folder          string `json:"folder"`
	Description     string `json:"description"`
	TransportType   string `json:"transport_type"`
	TransportConfig string `json:"transport_config"` // JSON-строка
	SyncPeriod      string `json:"sync_period"`
	SyncDirection   string `json:"sync_direction"`
	SyncConflict    string `json:"sync_conflict"`
	SyncSendFilter  string `json:"sync_send_filter"`
	SyncRecvFilter  string `json:"sync_recv_filter"`
}

// initConfigTables создаёт таблицы для конфигов, если их нет.
func initConfigTables(db *Journal) error {
	_, err := db.db.Exec(`
		CREATE TABLE IF NOT EXISTS folders (
			name TEXT PRIMARY KEY,
			path TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS sync_configs (
			name TEXT PRIMARY KEY,
			folder TEXT NOT NULL,
			description TEXT DEFAULT '',
			transport_type TEXT NOT NULL,
			transport_config TEXT DEFAULT '{}',
			sync_period TEXT DEFAULT '0',
			sync_direction TEXT DEFAULT 'bidirectional',
			sync_conflict TEXT DEFAULT 'newer_wins',
			sync_send_filter TEXT DEFAULT '',
			sync_recv_filter TEXT DEFAULT ''
		);
	`)
	return err
}

// ─── Folders ───────────────────────────────────────────────

// ListFolders возвращает все зарегистрированные папки.
func (j *Journal) ListFolders() ([]FolderRecord, error) {
	rows, err := j.db.Query("SELECT name, path FROM folders ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer rows.Close()

	var result []FolderRecord
	for rows.Next() {
		var r FolderRecord
		if err := rows.Scan(&r.Name, &r.Path); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// AddFolder добавляет папку.
func (j *Journal) AddFolder(name, path string) error {
	_, err := j.db.Exec(
		"INSERT OR REPLACE INTO folders (name, path) VALUES (?, ?)",
		name, path,
	)
	return err
}

// RemoveFolder удаляет папку по имени.
func (j *Journal) RemoveFolder(name string) error {
	_, err := j.db.Exec("DELETE FROM folders WHERE name = ?", name)
	return err
}

// ─── Sync Configs ──────────────────────────────────────────

// ListSyncConfigs возвращает все конфиги синхронизации.
func (j *Journal) ListSyncConfigs() ([]SyncConfigRecord, error) {
	rows, err := j.db.Query(`
		SELECT name, folder, description, transport_type, transport_config,
		       sync_period, sync_direction, sync_conflict,
		       sync_send_filter, sync_recv_filter
		FROM sync_configs ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list sync configs: %w", err)
	}
	defer rows.Close()

	var result []SyncConfigRecord
	for rows.Next() {
		var r SyncConfigRecord
		if err := rows.Scan(
			&r.Name, &r.Folder, &r.Description,
			&r.TransportType, &r.TransportConfig,
			&r.SyncPeriod, &r.SyncDirection, &r.SyncConflict,
			&r.SyncSendFilter, &r.SyncRecvFilter,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetSyncConfig возвращает один конфиг по имени.
func (j *Journal) GetSyncConfig(name string) (*SyncConfigRecord, error) {
	row := j.db.QueryRow(`
		SELECT name, folder, description, transport_type, transport_config,
		       sync_period, sync_direction, sync_conflict,
		       sync_send_filter, sync_recv_filter
		FROM sync_configs WHERE name = ?
	`, name)

	var r SyncConfigRecord
	err := row.Scan(
		&r.Name, &r.Folder, &r.Description,
		&r.TransportType, &r.TransportConfig,
		&r.SyncPeriod, &r.SyncDirection, &r.SyncConflict,
		&r.SyncSendFilter, &r.SyncRecvFilter,
	)
	if err != nil {
		return nil, fmt.Errorf("get sync config %q: %w", name, err)
	}
	return &r, nil
}

// AddSyncConfig добавляет или обновляет конфиг синхронизации.
// transportConfig — произвольная мапа, сериализуется в JSON.
func (j *Journal) AddSyncConfig(name string, record SyncConfigRecord) error {
	_, err := j.db.Exec(`
		INSERT OR REPLACE INTO sync_configs
			(name, folder, description, transport_type, transport_config,
			 sync_period, sync_direction, sync_conflict,
			 sync_send_filter, sync_recv_filter)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		name, record.Folder, record.Description,
		record.TransportType, record.TransportConfig,
		record.SyncPeriod, record.SyncDirection, record.SyncConflict,
		record.SyncSendFilter, record.SyncRecvFilter,
	)
	return err
}

// RemoveSyncConfig удаляет конфиг синхронизации.
func (j *Journal) RemoveSyncConfig(name string) error {
	_, err := j.db.Exec("DELETE FROM sync_configs WHERE name = ?", name)
	return err
}

// ─── Helpers ───────────────────────────────────────────────

// ClearAllFolders удаляет все папки.
func (j *Journal) ClearAllFolders() error {
	_, err := j.db.Exec("DELETE FROM folders")
	return err
}

// ClearAllSyncConfigs удаляет все конфиги синхронизации.
func (j *Journal) ClearAllSyncConfigs() error {
	_, err := j.db.Exec("DELETE FROM sync_configs")
	return err
}

// EncodeTransportConfig сериализует map[string]string в JSON.
func EncodeTransportConfig(cfg map[string]string) string {
	if len(cfg) == 0 {
		return "{}"
	}
	data, _ := json.Marshal(cfg)
	return string(data)
}

// DecodeTransportConfig десериализует JSON в map[string]string.
func DecodeTransportConfig(data string) map[string]string {
	if data == "" || data == "{}" {
		return map[string]string{}
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		return map[string]string{}
	}
	return cfg
}
