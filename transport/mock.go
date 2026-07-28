package transport

import (
	"fmt"
	"os"
	"path/filepath"
)

// Mock — тестовый транспорт, работающий с локальной директорией.
type Mock struct {
	root string
}

func NewMock(cfg map[string]string) (*Mock, error) {
	root := cfg["root"]
	if root == "" {
		return nil, fmt.Errorf("mock: root path required")
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("mock: cannot create root %s: %w", root, err)
	}
	return &Mock{root: root}, nil
}

func (m *Mock) Name() string { return "mock" }

func (m *Mock) List(remotePath string) ([]FileInfo, error) {
	full := filepath.Join(m.root, remotePath)
	var result []FileInfo

	err := filepath.Walk(full, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(m.root, path)
		if rel == "." {
			return nil
		}
		result = append(result, FileInfo{
			Name:    info.Name(),
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mock list %s: %w", remotePath, err)
	}
	return result, nil
}

func (m *Mock) Push(localPath, remotePath string) error {
	fullLocal := localPath
	fullRemote := filepath.Join(m.root, remotePath)
	if err := os.MkdirAll(filepath.Dir(fullRemote), 0755); err != nil {
		return fmt.Errorf("mock push mkdir: %w", err)
	}
	data, err := os.ReadFile(fullLocal)
	if err != nil {
		return fmt.Errorf("mock push read %s: %w", localPath, err)
	}
	if err := os.WriteFile(fullRemote, data, 0644); err != nil {
		return fmt.Errorf("mock push write %s: %w", remotePath, err)
	}
	return nil
}

func (m *Mock) Pull(remotePath, localPath string) error {
	fullRemote := filepath.Join(m.root, remotePath)
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("mock pull mkdir: %w", err)
	}
	data, err := os.ReadFile(fullRemote)
	if err != nil {
		return fmt.Errorf("mock pull read %s: %w", remotePath, err)
	}
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return fmt.Errorf("mock pull write %s: %w", localPath, err)
	}
	return nil
}

func (m *Mock) Delete(remotePath string) error {
	full := filepath.Join(m.root, remotePath)
	return os.RemoveAll(full)
}

func (m *Mock) Test() error {
	_, err := os.Stat(m.root)
	return err
}
