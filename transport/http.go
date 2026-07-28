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

// HTTPClient — транспорт для HTTP-хранилища через PHP-скрипт.
// Серверная часть: php_storage.php
//
// Конфиг:
//
//	url:      "https://myserver.com/php_storage.php"  — URL PHP-скрипта
//	base_url: "https://myserver.com"                   — базовый URL для прямых ссылок
//	auth:     "user:password"                          — Basic Auth (опционально)
//	self_signed_certs: "true"                          — разрешить самоподписанные сертификаты
type HTTPClient struct {
	url     string
	baseURL string
	auth    string
	client  *http.Client
}

// ErrNotSupported возвращается для операций, не поддерживаемых транспортом.
var ErrNotSupported = fmt.Errorf("operation not supported by this transport")

// fileEntry — элемент списка файлов от PHP-сервера.
type fileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

func NewHTTPClient(cfg map[string]string) (*HTTPClient, error) {
	url := cfg["url"]
	if url == "" {
		return nil, fmt.Errorf("http: url required")
	}

	baseURL := cfg["base_url"]
	if baseURL == "" {
		// Если base_url не указан, используем url без имени скрипта
		baseURL = url
		if idx := strings.LastIndex(baseURL, "/"); idx > 0 {
			baseURL = baseURL[:idx]
		}
	}

	tr := &http.Transport{}
	if cfg["self_signed_certs"] == "true" {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &HTTPClient{
		url:     url,
		baseURL: strings.TrimRight(baseURL, "/"),
		auth:    cfg["auth"],
		client:  &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}, nil
}

func (h *HTTPClient) Name() string { return "http" }

// setAuth добавляет Basic Auth заголовок если настроен.
func (h *HTTPClient) setAuth(req *http.Request) {
	if h.auth != "" {
		parts := strings.SplitN(h.auth, ":", 2)
		if len(parts) == 2 {
			req.SetBasicAuth(parts[0], parts[1])
		}
	}
}

// List возвращает список файлов из HTTP-хранилища.
// remotePath игнорируется — хранилище плоское, без подпапок.
func (h *HTTPClient) List(remotePath string) ([]FileInfo, error) {
	req, err := http.NewRequest("GET", h.url, nil)
	if err != nil {
		return nil, fmt.Errorf("http list req: %w", err)
	}
	h.setAuth(req)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http list: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http list read: %w", err)
	}

	var entries []fileEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("http list parse: %w", err)
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

// Push загружает файл в HTTP-хранилище через POST multipart/form-data.
func (h *HTTPClient) Push(localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("http push open %s: %w", localPath, err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return fmt.Errorf("http push form: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("http push copy: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", h.url, &buf)
	if err != nil {
		return fmt.Errorf("http push req: %w", err)
	}
	h.setAuth(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("http push: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http push: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Pull скачивает файл из HTTP-хранилища.
// remotePath — имя файла (как получено из List).
func (h *HTTPClient) Pull(remotePath, localPath string) error {
	downloadURL := h.baseURL + "/uploads/" + remotePath

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("http pull req: %w", err)
	}
	h.setAuth(req)

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("http pull: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("http pull: status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("http pull mkdir: %w", err)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("http pull create %s: %w", localPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("http pull copy: %w", err)
	}
	return nil
}

// Delete не поддерживается (сервер append-only).
func (h *HTTPClient) Delete(remotePath string) error {
	return ErrNotSupported
}

// Test проверяет доступность HTTP-хранилища.
func (h *HTTPClient) Test() error {
	_, err := h.List("")
	return err
}
