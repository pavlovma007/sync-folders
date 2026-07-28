package core

import "time"

// FileInfo представляет файл в папке синхронизации.
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

// ConflictMode определяет поведение при конфликте.
type ConflictMode string

const (
	ConflictNewerWins ConflictMode = "newer_wins"
	ConflictKeepBoth  ConflictMode = "keep_both"
)

// Direction определяет направление синхронизации.
type Direction string

const (
	DirectionPush          Direction = "push"
	DirectionPull          Direction = "pull"
	DirectionBidirectional Direction = "bidirectional"
)

// SyncConfig описывает конфиг синхронизации (один YAML файл).
type SyncConfig struct {
	Folder      string          `yaml:"folder"`
	Description string          `yaml:"description"`
	Transport   TransportConfig `yaml:"transport"`
	Sync        SyncSettings    `yaml:"sync"`
}

type TransportConfig struct {
	Type   string            `yaml:"type"`
	Config map[string]string `yaml:"config"`
}

type SyncSettings struct {
	Period        string       `yaml:"period"`
	Direction     Direction    `yaml:"direction"`
	SendFilter    string       `yaml:"send_filter"`
	ReceiveFilter string       `yaml:"receive_filter"`
	Conflict      ConflictMode `yaml:"conflict"`
}

// Folder представляет зарегистрированную локальную папку.
type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Config хранит состояние приложения (папки и конфиги).
type Config struct {
	Folders []Folder              `json:"folders"`
	Syncs   map[string]SyncConfig `json:"syncs"` // key = config name
}
