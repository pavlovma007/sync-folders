package transport

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MySQLClient — транспорт для MySQL-хранилища через PHP-скрипт.
// Серверная часть: mysql_storage.php
//
// Конфиг:
//
//	url:   "https://myserver.com/mysql_storage.php"  — URL PHP-скрипта
//	group: "my-sync"                                  — группа файлов (опционально)
//	auth:  "user:password"                            — Basic Auth (опционально)
//	self_signed_certs: "true"                         — самоподписанные сертификаты
type MySQLClient struct {
	url    string
	group  string
	auth   string
	client *http.Client
}

// mysqlFileEntry — элемент списка от PHP-сервера.
type mysqlFileEntry struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

func NewMySQLClient(cfg map[string]string) (*MySQLClient, error) {
	url := cfg["url"]
	if url == "" {
		return nil, fmt.Errorf("mysql: url required")
	}

	group := cfg["group"]
	if group == "" {
		group = "default_group"
	}

	tr := &http.Transport{}
	if cfg["self_signed_certs"] == "true" {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &MySQLClient{
		url:    url,
		group:  group,
		auth:   cfg["auth"],
		client: &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}, nil
}

func (m *MySQLClient) Name() string { return "mysql" }

func (m *MySQLClient) setAuth(req *http.Request) {
	if m.auth != "" {
		parts := strings.SplitN(m.auth, ":", 2)
		if len(parts) == 2 {
			req.SetBasicAuth(parts[0], parts[1])
		}
	}
}

// List возвращает список файлов. Если задана группа — фильтрует по ней.
func (m *MySQLClient) List(remotePath string) ([]FileInfo, error) {
	urlStr := m.url + "?group=" + m.group
	if remotePath != "" {
		urlStr += "&file_name=" + remotePath
	}

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("mysql list req: %w", err)
	}
	m.setAuth(req)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mysql list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("mysql list: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mysql list read: %w", err)
	}

	// Пустой ответ (нет файлов) — валидный JSON массив
	if string(body) == "null" || string(body) == "" {
		return []FileInfo{}, nil
	}

	var entries []mysqlFileEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("mysql list parse: %w", err)
	}

	var result []FileInfo
	for _, e := range entries {
		result = append(result, FileInfo{
			Name:    e.Name,
			Path:    e.Name,
			Size:    e.Size,
			ModTime: time.Unix(e.ModTime, 0),
			IsDir:   false,
		})
	}
	return result, nil
}

// Push загружает файл в MySQL-хранилище.
func (m *MySQLClient) Push(localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("mysql push open %s: %w", localPath, err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Поле file
	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return fmt.Errorf("mysql push form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("mysql push copy: %w", err)
	}

	// Поле group
	if err := writer.WriteField("group", m.group); err != nil {
		return fmt.Errorf("mysql push group: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", m.url, &buf)
	if err != nil {
		return fmt.Errorf("mysql push req: %w", err)
	}
	m.setAuth(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mysql push: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mysql push: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Pull скачивает файл из MySQL-хранилища по имени.
func (m *MySQLClient) Pull(remotePath, localPath string) error {
	urlStr := m.url + "?file_name=" + remotePath

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return fmt.Errorf("mysql pull req: %w", err)
	}
	m.setAuth(req)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mysql pull: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("mysql pull: status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("mysql pull mkdir: %w", err)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("mysql pull create %s: %w", localPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("mysql pull copy: %w", err)
	}
	return nil
}

// Delete не поддерживается (сервер append-only).
func (m *MySQLClient) Delete(remotePath string) error {
	return ErrNotSupported
}

// Test проверяет доступность MySQL-хранилища.
func (m *MySQLClient) Test() error {
	_, err := m.List("")
	return err
}
