package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigPath — путь к служебной БД приложения.
const ConfigPath = ".config/sync-app"

// AppDBPath возвращает путь к SQLite файлу.
func AppDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.db"), nil
}

// LoadConfig загружает состояние из JSON файла (рядом с БД).
func LoadConfig() (*Config, error) {
	dbPath, err := AppDBPath()
	if err != nil {
		return nil, err
	}
	cfgPath := filepath.Dir(dbPath) + "/state.json"

	cfg := &Config{
		Folders: []Folder{},
		Syncs:   map[string]SyncConfig{},
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // новый, пустой
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	return cfg, nil
}

// SaveConfig сохраняет состояние.
func SaveConfig(cfg *Config) error {
	dbPath, err := AppDBPath()
	if err != nil {
		return err
	}
	cfgPath := filepath.Dir(dbPath) + "/state.json"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

// AddFolder добавляет папку.
func AddFolder(name, path string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.Folders = append(cfg.Folders, Folder{Name: name, Path: path})
	return SaveConfig(cfg)
}

// RemoveFolder удаляет папку.
func RemoveFolder(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	var kept []Folder
	for _, f := range cfg.Folders {
		if f.Name != name {
			kept = append(kept, f)
		}
	}
	cfg.Folders = kept
	return SaveConfig(cfg)
}

// ListFolders возвращает список папок.
func ListFolders() ([]Folder, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return cfg.Folders, nil
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

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Syncs == nil {
		cfg.Syncs = map[string]SyncConfig{}
	}
	// имя конфига = имя файла без .yaml
	name := filepath.Base(yamlPath)
	name = name[:len(name)-len(filepath.Ext(name))]
	cfg.Syncs[name] = sc
	return SaveConfig(cfg)
}

// RemoveConfig удаляет конфиг по имени.
func RemoveConfig(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	delete(cfg.Syncs, name)
	return SaveConfig(cfg)
}

// ListConfigs возвращает все конфиги.
func ListConfigs() (map[string]SyncConfig, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return cfg.Syncs, nil
}
