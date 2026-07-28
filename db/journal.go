package db

import (
	"database/sql"
	"fmt"
	"sync-folders/core"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Journal хранит историю синхронизации.
type Journal struct {
	db *sql.DB
}

// Open открывает или создаёт БД журнала.
func Open() (*Journal, error) {
	path, err := core.AppDBPath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("journal open: %w", err)
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
	return &Journal{db: db}, nil
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

// Close закрывает БД.
func (j *Journal) Close() error {
	return j.db.Close()
}
