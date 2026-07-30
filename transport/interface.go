package transport

import (
	"fmt"
	"time"
)

// FileInfo представляет файл в папке (дублирует core.FileInfo, чтобы избежать цикла).
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

// Transport — интерфейс для любого способа синхронизации.
type Transport interface {
	Name() string
	List(remotePath string) ([]FileInfo, error)
	Push(localPath, remotePath string) error
	Pull(remotePath, localPath string) error
	Delete(remotePath string) error
	Test() error
}

// Factory создаёт транспорт по типу и конфигу.
func Factory(typ string, cfg map[string]string) (Transport, error) {
	switch typ {
	case "mock":
		return NewMock(cfg)
	case "ssh":
		return NewSSH(cfg)
	case "ftp":
		return NewFTP(cfg)
	case "webdav":
		return NewWebDAV(cfg)
	case "s3":
		return NewS3(cfg)
	case "http":
		return NewHTTPClient(cfg)
	case "email":
		return NewEmailClient(cfg)
	case "mysql":
		return NewMySQLClient(cfg)
	case "ipfs":
		return NewIPFSClient(cfg)
	case "torrent":
		return newTorrentFromConfig(cfg)
	default:
		return nil, fmt.Errorf("unknown transport: %s", typ)
	}
}
