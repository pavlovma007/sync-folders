package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	if err := j.InitHeartbeats(); err != nil {
		return nil, fmt.Errorf("heartbeats init: %w", err)
	}

	// Очистка устаревших записей журнала и heartbeat'ов
	j.cleanOldLogs()

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

// ─── Heartbeats ─────────────────────────────────────────────

// ActiveProcess представляет живой процесс (пишущий heartbeat в БД).
type ActiveProcess struct {
	PID     int    `json:"pid"`
	Role    string `json:"role"`
	Config  string `json:"config"`
	Started int64  `json:"started"`
	Uptime  int64  `json:"uptime"`
}

// InitHeartbeats создаёт таблицу heartbeats, если её нет.
func (j *Journal) InitHeartbeats() error {
	_, err := j.db.Exec(`CREATE TABLE IF NOT EXISTS heartbeats (
		pid       INTEGER PRIMARY KEY,
		role      TEXT,
		config    TEXT,
		started   INTEGER,
		last_beat INTEGER
	)`)
	return err
}

// Heartbeat обновляет запись живого процесса. При первом вызове
// сохраняет время старта, при повторных — только время последнего heartbeat.
func (j *Journal) Heartbeat(pid int, role, configName string) error {
	now := time.Now().Unix()
	_, err := j.db.Exec(`INSERT OR REPLACE INTO heartbeats (pid, role, config, started, last_beat)
		VALUES (?, ?, ?, COALESCE((SELECT started FROM heartbeats WHERE pid=?), ?), ?)`,
		pid, role, configName, pid, now, now)
	return err
}

// SetDaemonInfo сохраняет pid и порт HTTP-демона (для CLI-делегирования DHT).
// Использует таблицу settings (key-value), как и остальные настройки.
func (j *Journal) SetDaemonInfo(pid, port int) error {
	if err := j.SetSetting("daemon_pid", strconv.Itoa(pid)); err != nil {
		return err
	}
	return j.SetSetting("daemon_port", strconv.Itoa(port))
}

// GetDaemonInfo возвращает pid и порт демона (0, 0 — если не записано).
func (j *Journal) GetDaemonInfo() (pid, port int) {
	pid, _ = strconv.Atoi(j.GetSetting("daemon_pid"))
	port, _ = strconv.Atoi(j.GetSetting("daemon_port"))
	return
}

// ClearDaemonInfo удаляет информацию о демоне (при его остановке).
func (j *Journal) ClearDaemonInfo() error {
	_, err := j.db.Exec("DELETE FROM settings WHERE key IN ('daemon_pid', 'daemon_port')")
	return err
}

// GetActiveProcesses возвращает процессы, heartbeat которых был недавно (< 90 сек).
func (j *Journal) GetActiveProcesses() []ActiveProcess {
	now := time.Now().Unix()
	rows, err := j.db.Query(`SELECT pid, role, config, started FROM heartbeats WHERE last_beat > ?`, now-90)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []ActiveProcess
	for rows.Next() {
		var a ActiveProcess
		if err := rows.Scan(&a.PID, &a.Role, &a.Config, &a.Started); err != nil {
			continue
		}
		a.Uptime = now - a.Started
		result = append(result, a)
	}
	return result
}

// ─── Очистка журнала ────────────────────────────────────────

// cleanOldLogs удаляет записи sync_log старше 30 дней и устаревшие heartbeats.
func (j *Journal) cleanOldLogs() {
	cutoff := time.Now().Add(-30 * 24 * time.Hour).Unix()
	j.db.Exec("DELETE FROM sync_log WHERE ts < ?", cutoff)
	j.db.Exec("DELETE FROM heartbeats WHERE last_beat < ?", time.Now().Unix()-90)
}

// ─── Выборки для Web UI ─────────────────────────────────────

// SyncLogEntry представляет запись журнала синхронизации.
type SyncLogEntry struct {
	ConfigName string `json:"config_name"`
	FilePath   string `json:"file_path"`
	Direction  string `json:"direction"`
	Size       int64  `json:"size"`
	Status     string `json:"status"`
	TS         int64  `json:"ts"`
}

// GetJournalTail возвращает последние n записей журнала.
func (j *Journal) GetJournalTail(n int) []SyncLogEntry {
	rows, err := j.db.Query(
		"SELECT config_name, file_path, direction, size, status, ts FROM sync_log ORDER BY ts DESC LIMIT ?", n)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []SyncLogEntry
	for rows.Next() {
		var e SyncLogEntry
		if err := rows.Scan(&e.ConfigName, &e.FilePath, &e.Direction, &e.Size, &e.Status, &e.TS); err != nil {
			continue
		}
		result = append(result, e)
	}
	return result
}

// RecentFile представляет недавно синхронизированный файл.
type RecentFile struct {
	FilePath string `json:"file_path"`
	LastSync int64  `json:"last_sync"`
	Ops      int    `json:"ops"`
}

// GetRecentFiles возвращает n файлов, отсортированных по времени последней синхронизации.
func (j *Journal) GetRecentFiles(n int) []RecentFile {
	rows, err := j.db.Query(`SELECT file_path, MAX(ts) as last_sync, COUNT(*) as ops
		FROM sync_log GROUP BY file_path ORDER BY last_sync DESC LIMIT ?`, n)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []RecentFile
	for rows.Next() {
		var r RecentFile
		if err := rows.Scan(&r.FilePath, &r.LastSync, &r.Ops); err != nil {
			continue
		}
		result = append(result, r)
	}
	return result
}

// Close закрывает БД.
func (j *Journal) Close() error {
	return j.db.Close()
}
