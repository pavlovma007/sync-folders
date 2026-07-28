package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Journal хранит историю синхронизации.
type Journal struct {
	db *sql.DB
}

// appDBPath возвращает путь к SQLite файлу приложения.
func appDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "sync-app")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.db"), nil
}

// Open открывает или создаёт БД приложения.
func Open() (*Journal, error) {
	path, err := appDBPath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sync_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		config_name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		direction TEXT NOT NULL,
		size INTEGER DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'ok',
		error TEXT DEFAULT '',
		ts INTEGER NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("journal init: %w", err)
	}

	// Инициализация таблиц конфигов
	j := &Journal{db: db}
	if err := initConfigTables(j); err != nil {
		return nil, fmt.Errorf("config tables init: %w", err)
	}

	return j, nil
}

// Log записывает событие синхронизации.
func (j *Journal) Log(configName, filePath, direction string, size int64, err error) error {
	status := "ok"
	errStr := ""
	if err != nil {
		status = "error"
		errStr = err.Error()
	}
	_, err2 := j.db.Exec(
		"INSERT INTO sync_log (config_name, file_path, direction, size, status, error, ts) VALUES (?, ?, ?, ?, ?, ?, ?)",
		configName, filePath, direction, size, status, errStr, time.Now().Unix(),
	)
	return err2
}

// LastSync возвращает время последней успешной синхронизации для конфига.
// Возвращает zero time, если синхронизаций не было.
func (j *Journal) LastSync(configName string) (time.Time, error) {
	var ts int64
	err := j.db.QueryRow(
		"SELECT MAX(ts) FROM sync_log WHERE config_name = ? AND status = 'ok'",
		configName,
	).Scan(&ts)
	if err == sql.ErrNoRows || ts == 0 {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts, 0), nil
}

// LastSyncAll возвращает время последней успешной синхронизации для всех конфигов.
func (j *Journal) LastSyncAll() (map[string]time.Time, error) {
	rows, err := j.db.Query(
		"SELECT config_name, MAX(ts) FROM sync_log WHERE status = 'ok' GROUP BY config_name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var name string
		var ts int64
		if err := rows.Scan(&name, &ts); err != nil {
			continue
		}
		result[name] = time.Unix(ts, 0)
	}
	return result, rows.Err()
}

// LastError возвращает последнюю ошибку для конфига.
func (j *Journal) LastError(configName string) (string, time.Time, error) {
	var errStr string
	var ts int64
	err := j.db.QueryRow(
		"SELECT error, ts FROM sync_log WHERE config_name = ? AND status = 'error' ORDER BY ts DESC LIMIT 1",
		configName,
	).Scan(&errStr, &ts)
	if err == sql.ErrNoRows {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, err
	}
	return errStr, time.Unix(ts, 0), nil
}

// Close закрывает БД.
func (j *Journal) Close() error {
	return j.db.Close()
}
