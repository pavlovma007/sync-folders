package core

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync-folders/filter"
	"sync-folders/transport"
	"time"
)

// SyncEngine выполняет синхронизацию по одному конфигу.
type SyncEngine struct {
	config     SyncConfig
	localPath  string
	transp     transport.Transport
	sendFilter *filter.Engine
	recvFilter *filter.Engine
}

// NewSyncEngine создаёт движок для конфига.
func NewSyncEngine(name string, sc SyncConfig) (*SyncEngine, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	var localPath string
	for _, f := range cfg.Folders {
		if f.Name == sc.Folder {
			localPath = f.Path
			break
		}
	}
	if localPath == "" {
		return nil, fmt.Errorf("folder %q not registered", sc.Folder)
	}

	transp, err := transport.Factory(sc.Transport.Type, sc.Transport.Config)
	if err != nil {
		return nil, fmt.Errorf("transport: %w", err)
	}

	e := &SyncEngine{
		config:    sc,
		localPath: localPath,
		transp:    transp,
	}

	if sc.Sync.SendFilter != "" {
		e.sendFilter = filter.New("send", sc.Sync.SendFilter)
	}
	if sc.Sync.ReceiveFilter != "" {
		e.recvFilter = filter.New("receive", sc.Sync.ReceiveFilter)
	}

	return e, nil
}

// RunOnce выполняет один цикл синхронизации.
func (e *SyncEngine) RunOnce() error {
	log.Printf("[sync] %s: start (dir=%s, transport=%s, direction=%s)",
		e.config.Folder, e.localPath, e.config.Transport.Type, e.config.Sync.Direction)

	switch e.config.Sync.Direction {
	case DirectionPush:
		return e.push()
	case DirectionPull:
		return e.pull()
	case DirectionBidirectional:
		if err := e.push(); err != nil {
			return err
		}
		return e.pull()
	}
	return nil
}

func (e *SyncEngine) push() error {
	files, err := e.listLocal("")
	if err != nil {
		return err
	}

	if e.sendFilter != nil {
		files, err = e.sendFilter.Run(files, e.localPath, "send")
		if err != nil {
			return fmt.Errorf("send filter: %w", err)
		}
	}

	for _, f := range files {
		if f.IsDir {
			continue
		}
		fullLocalPath := filepath.Join(e.localPath, f.Path)
		log.Printf("  push: %s", f.Path)
		if err := e.transp.Push(fullLocalPath, f.Path); err != nil {
			log.Printf("  push error: %v", err)
		}
	}
	return nil
}

func (e *SyncEngine) pull() error {
	remoteFiles, err := e.transp.List("")
	if err != nil {
		return fmt.Errorf("remote list: %w", err)
	}

	if e.recvFilter != nil {
		remoteFiles, err = e.recvFilter.Run(remoteFiles, e.localPath, "receive")
		if err != nil {
			return fmt.Errorf("receive filter: %w", err)
		}
	}

	for _, f := range remoteFiles {
		if f.IsDir {
			continue
		}
		localPath := filepath.Join(e.localPath, f.Path)
		log.Printf("  pull: %s", f.Path)
		if err := e.transp.Pull(f.Path, localPath); err != nil {
			log.Printf("  pull error: %v", err)
		}
	}
	return nil
}

func (e *SyncEngine) listLocal(prefix string) ([]transport.FileInfo, error) {
	var result []transport.FileInfo
	root := filepath.Join(e.localPath, prefix)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(e.localPath, path)
		if rel == "." {
			return nil
		}
		result = append(result, transport.FileInfo{
			Name:    info.Name(),
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})
		return nil
	})
	return result, err
}

func toTransportFiles(files []FileInfo) []transport.FileInfo {
	var r []transport.FileInfo
	for _, f := range files {
		r = append(r, transport.FileInfo{
			Name:    f.Name,
			Path:    f.Path,
			Size:    f.Size,
			ModTime: f.ModTime,
			IsDir:   f.IsDir,
		})
	}
	return r
}

// Daemon запускает периодическую синхронизацию всех конфигов.
func Daemon(interval time.Duration) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	for name, sc := range cfg.Syncs {
		engine, err := NewSyncEngine(name, sc)
		if err != nil {
			log.Printf("[daemon] init %s: %v", name, err)
			continue
		}
		if err := engine.RunOnce(); err != nil {
			log.Printf("[daemon] sync %s: %v", name, err)
		}
	}

	if interval == 0 {
		return nil
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		cfg, err := LoadConfig()
		if err != nil {
			continue
		}
		for name, sc := range cfg.Syncs {
			engine, err := NewSyncEngine(name, sc)
			if err != nil {
				continue
			}
			engine.RunOnce()
		}
	}
	return nil
}
