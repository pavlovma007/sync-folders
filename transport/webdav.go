package transport

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type WebDAVClient struct {
	baseURL  string
	user     string
	password string
	rootPath string
	client   *http.Client
}

func NewWebDAV(cfg map[string]string) (*WebDAVClient, error) {
	return &WebDAVClient{
		baseURL:  cfg["url"],
		user:     cfg["user"],
		password: cfg["password"],
		rootPath: cfg["remote_path"],
		client:   &http.Client{},
	}, nil
}

func (w *WebDAVClient) Name() string { return "webdav" }

func (w *WebDAVClient) url(p string) string {
	return strings.TrimRight(w.baseURL, "/") + "/" + strings.TrimLeft(path.Join(w.rootPath, p), "/")
}

func (w *WebDAVClient) List(remotePath string) ([]FileInfo, error) {
	// Simple WebDAV PROPFIND
	req, err := http.NewRequest("PROPFIND", w.url(remotePath), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(w.user, w.password)
	req.Header.Set("Depth", "1")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webdav propfind: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 207 {
		return nil, fmt.Errorf("webdav: unexpected status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	_ = body // TODO: parse XML response
	return nil, nil
}

func (w *WebDAVClient) Push(localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	req, err := http.NewRequest("PUT", w.url(remotePath), file)
	if err != nil {
		return err
	}
	req.SetBasicAuth(w.user, w.password)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webdav put: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav put: status %d", resp.StatusCode)
	}
	return nil
}

func (w *WebDAVClient) Pull(remotePath, localPath string) error {
	req, err := http.NewRequest("GET", w.url(remotePath), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(w.user, w.password)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webdav get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("webdav get: status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (w *WebDAVClient) Delete(remotePath string) error {
	req, err := http.NewRequest("DELETE", w.url(remotePath), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(w.user, w.password)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webdav delete: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (w *WebDAVClient) Test() error {
	_, err := w.List("")
	return err
}
