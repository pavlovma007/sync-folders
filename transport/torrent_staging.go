package transport

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// Staging управляет временной директорией для накопления файлов перед snapshot.
type Staging struct {
	rootDir string // .sync-torrent-staging/
	latest  string // .sync-torrent-staging/latest/
}

// FileEntry — одна запись в NDJSON-манифесте.
type FileEntry struct {
	Path   string `json:"p"`
	Size   int64  `json:"s"`
	Mod    int64  `json:"m"` // unix timestamp
	SHA256 string `json:"h"`
}

// Snapshot содержит результат сборки торрента.
type Snapshot struct {
	TorrentData []byte // .torrent файл
	Magnet      string // magnet:?xt=urn:btih:<infohash>
	InfoHash    string // hex info_hash
	Files       []FileEntry
}

// StagingManifest — манифест опубликованного снапшота.
type StagingManifest struct {
	Seq       int64       `json:"seq"`
	Timestamp int64       `json:"ts"`
	FilesHash string      `json:"files_hash"`
	Files     []FileEntry `json:"-"`
}

// NewStaging создаёт staging-директорию.
func NewStaging(rootDir string) *Staging {
	latest := filepath.Join(rootDir, "latest")
	return &Staging{rootDir: rootDir, latest: latest}
}

// Add копирует файл из srcPath в staging под remotePath.
func (s *Staging) Add(srcPath, remotePath string) error {
	dest := filepath.Join(s.latest, filepath.FromSlash(remotePath))
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("staging mkdir: %w", err)
	}
	return copyFile(srcPath, dest)
}

// HasChanges сравнивает staging/latest/ с последним сохранённым манифестом.
func (s *Staging) HasChanges() (bool, error) {
	lastManifestPath := filepath.Join(s.rootDir, ".torrent-last-manifest.ndjson")
	if _, err := os.Stat(lastManifestPath); os.IsNotExist(err) {
		return true, nil // нет предыдущего манифеста → есть изменения
	}

	currentFiles, err := s.scanStaging()
	if err != nil {
		return false, fmt.Errorf("scan staging: %w", err)
	}

	lastFiles, err := readLastManifest(lastManifestPath)
	if err != nil {
		return true, nil // не можем прочитать → считаем что есть изменения
	}

	if len(currentFiles) != len(lastFiles) {
		return true, nil
	}
	for i, cf := range currentFiles {
		if i >= len(lastFiles) {
			return true, nil
		}
		if cf.Path != lastFiles[i].Path || cf.SHA256 != lastFiles[i].SHA256 {
			return true, nil
		}
	}
	return false, nil
}

// BuildSnapshot создаёт .torrent из staging/latest/ и возвращает snapshot.
func (s *Staging) BuildSnapshot(project string) (*Snapshot, *StagingManifest, error) {
	files, err := s.scanStaging()
	if err != nil {
		return nil, nil, fmt.Errorf("scan staging: %w", err)
	}

	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no files in staging")
	}

	// Создаём metainfo
	mi := metainfo.MetaInfo{}
	mi.SetDefaults()
	mi.CreatedBy = "sync-folders"

	info := metainfo.Info{
		PieceLength: 512 * 1024, // 512KB pieces
		Name:        project,
	}

	// Добавляем файлы
	for _, f := range files {
		pathParts := strings.Split(filepath.ToSlash(f.Path), "/")
		info.Files = append(info.Files, metainfo.FileInfo{
			Path:   pathParts,
			Length: f.Size,
		})
	}

	// Генерируем pieces
	if err := info.GeneratePieces(func(fi metainfo.FileInfo) (io.ReadCloser, error) {
		fullPath := filepath.Join(s.latest, filepath.FromSlash(strings.Join(fi.Path, string(filepath.Separator))))
		return os.Open(fullPath)
	}); err != nil {
		return nil, nil, fmt.Errorf("generate pieces: %w", err)
	}

	// Маршалим info в bencode и устанавливаем как InfoBytes
	var infoBuf bytes.Buffer
	if err := bencode.NewEncoder(&infoBuf).Encode(info); err != nil {
		return nil, nil, fmt.Errorf("bencode info: %w", err)
	}
	mi.InfoBytes = infoBuf.Bytes()

	// Получаем info_hash и magnet
	infoHash := mi.HashInfoBytes()
	magnet := mi.Magnet(&infoHash, &info).String()
	if magnet == "" {
		magnet = "magnet:?xt=urn:btih:" + infoHash.HexString()
	}

	// Полный .torrent файл
	var torrentBuf bytes.Buffer
	if err := mi.Write(&torrentBuf); err != nil {
		return nil, nil, fmt.Errorf("write metainfo: %w", err)
	}

	// Build files_hash
	filesHash := hashFiles(files)

	manifest := &StagingManifest{
		Seq:       time.Now().Unix(),
		Timestamp: time.Now().Unix(),
		FilesHash: filesHash,
		Files:     files,
	}

	return &Snapshot{
		TorrentData: torrentBuf.Bytes(),
		Magnet:      magnet,
		InfoHash:    infoHash.HexString(),
		Files:       files,
	}, manifest, nil
}

// SaveLastManifest сохраняет манифест для будущих diff.
func (s *Staging) SaveLastManifest(m *StagingManifest) error {
	path := filepath.Join(s.rootDir, ".torrent-last-manifest.ndjson")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	defer f.Close()

	for _, fe := range m.Files {
		line, err := json.Marshal(fe)
		if err != nil {
			return fmt.Errorf("marshal file entry: %w", err)
		}
		if _, err := fmt.Fprintln(f, string(line)); err != nil {
			return fmt.Errorf("write manifest line: %w", err)
		}
	}
	return nil
}

// Clear очищает staging/latest/.
func (s *Staging) Clear() error {
	return os.RemoveAll(s.latest)
}

// scanStaging обходит staging/latest/ и строит FileEntry со sha256.
func (s *Staging) scanStaging() ([]FileEntry, error) {
	var files []FileEntry
	root := s.latest

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return files, nil
	}

	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}

		h, err := fileSHA256(path)
		if err != nil {
			return err
		}

		files = append(files, FileEntry{
			Path:   filepath.ToSlash(rel),
			Size:   fi.Size(),
			Mod:    fi.ModTime().Unix(),
			SHA256: h,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

// readLastManifest читает NDJSON-манифест построчно.
func readLastManifest(path string) ([]FileEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var files []FileEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var fe FileEntry
		if err := json.Unmarshal(line, &fe); err != nil {
			return nil, fmt.Errorf("parse manifest line: %w", err)
		}
		files = append(files, fe)
	}
	return files, scanner.Err()
}

// fileSHA256 вычисляет SHA256 хеш файла.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashFiles вычисляет агрегированный хеш списка файлов.
func hashFiles(files []FileEntry) string {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.Path))
		h.Write([]byte(f.SHA256))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// copyFile копирует файл из src в dst, создавая директории.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
