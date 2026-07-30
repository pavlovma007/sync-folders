package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// QBClient — клиент для qBittorrent Web API.
type QBClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
	cookies  []*http.Cookie
}

// NewQBClient создаёт клиент для qBittorrent.
func NewQBClient(baseURL, username, password string) *QBClient {
	return &QBClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// login аутентифицируется в qBittorrent Web API.
func (q *QBClient) login() error {
	if q.username == "" {
		return nil
	}
	v := url.Values{}
	v.Set("username", q.username)
	v.Set("password", q.password)

	resp, err := q.client.PostForm(q.baseURL+"/api/v2/auth/login", v)
	if err != nil {
		return fmt.Errorf("qb login: %w", err)
	}
	defer resp.Body.Close()

	q.cookies = resp.Cookies()
	if len(q.cookies) == 0 {
		return fmt.Errorf("qb login: no auth cookie received")
	}
	return nil
}

// doWithAuth выполняет запрос с авторизацией (логинится при первом вызове).
func (q *QBClient) doWithAuth(req *http.Request) (*http.Response, error) {
	if q.cookies == nil && q.username != "" {
		if err := q.login(); err != nil {
			return nil, err
		}
	}
	for _, c := range q.cookies {
		req.AddCookie(c)
	}
	return q.client.Do(req)
}

func (q *QBClient) Name() string { return "qbittorrent" }

func (q *QBClient) AddMagnet(magnetURI, savePath string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("urls", magnetURI); err != nil {
		return "", fmt.Errorf("qb add magnet write field: %w", err)
	}
	if savePath != "" {
		if err := w.WriteField("savepath", savePath); err != nil {
			return "", fmt.Errorf("qb add magnet write savepath: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("qb add magnet close: %w", err)
	}

	req, _ := http.NewRequest("POST", q.baseURL+"/api/v2/torrents/add", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := q.doWithAuth(req)
	if err != nil {
		return "", fmt.Errorf("qb add magnet: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("qb add magnet: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return "", nil
}

func (q *QBClient) AddTorrentFile(data []byte, savePath string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("torrents", "snapshot.torrent")
	if err != nil {
		return "", fmt.Errorf("qb add file form: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("qb add file write: %w", err)
	}
	if savePath != "" {
		if err := w.WriteField("savepath", savePath); err != nil {
			return "", fmt.Errorf("qb add file write savepath: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("qb add file close: %w", err)
	}

	req, _ := http.NewRequest("POST", q.baseURL+"/api/v2/torrents/add", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := q.doWithAuth(req)
	if err != nil {
		return "", fmt.Errorf("qb add file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("qb add file: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return "", nil
}

type qbTorrentInfo struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	Progress  float64 `json:"progress"`
	State     string  `json:"state"`
	SavePath  string  `json:"save_path"`
	TotalSize int64   `json:"total_size"`
}

func (q *QBClient) List() ([]TorrentInfo, error) {
	req, _ := http.NewRequest("GET", q.baseURL+"/api/v2/torrents/info", nil)
	resp, err := q.doWithAuth(req)
	if err != nil {
		return nil, fmt.Errorf("qb list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("qb list: status %d", resp.StatusCode)
	}

	var qbList []qbTorrentInfo
	if err := json.NewDecoder(resp.Body).Decode(&qbList); err != nil {
		return nil, fmt.Errorf("qb list decode: %w", err)
	}

	result := make([]TorrentInfo, len(qbList))
	for i, ti := range qbList {
		result[i] = TorrentInfo{
			Hash:     ti.Hash,
			Name:     ti.Name,
			Progress: ti.Progress,
			State:    mapQBState(ti.State),
			SavePath: ti.SavePath,
			Size:     ti.TotalSize,
		}
	}
	return result, nil
}

func (q *QBClient) GetInfo(hash string) (TorrentInfo, error) {
	u := q.baseURL + "/api/v2/torrents/info?hashes=" + url.QueryEscape(hash)
	req, _ := http.NewRequest("GET", u, nil)
	resp, err := q.doWithAuth(req)
	if err != nil {
		return TorrentInfo{}, fmt.Errorf("qb info: %w", err)
	}
	defer resp.Body.Close()

	var qbList []qbTorrentInfo
	if err := json.NewDecoder(resp.Body).Decode(&qbList); err != nil {
		return TorrentInfo{}, fmt.Errorf("qb info decode: %w", err)
	}
	if len(qbList) == 0 {
		return TorrentInfo{}, fmt.Errorf("qb info: torrent %q not found", hash)
	}
	ti := qbList[0]
	return TorrentInfo{
		Hash:     ti.Hash,
		Name:     ti.Name,
		Progress: ti.Progress,
		State:    mapQBState(ti.State),
		SavePath: ti.SavePath,
		Size:     ti.TotalSize,
	}, nil
}

func (q *QBClient) Delete(hash string, deleteFiles bool) error {
	v := url.Values{}
	v.Set("hashes", hash)
	if deleteFiles {
		v.Set("deleteFiles", "true")
	}
	req, _ := http.NewRequest("POST", q.baseURL+"/api/v2/torrents/delete", strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := q.doWithAuth(req)
	if err != nil {
		return fmt.Errorf("qb delete: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("qb delete: status %d", resp.StatusCode)
	}
	return nil
}

func (q *QBClient) Test() error {
	req, _ := http.NewRequest("GET", q.baseURL+"/api/v2/app/version", nil)
	resp, err := q.doWithAuth(req)
	if err != nil {
		return fmt.Errorf("qb test: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("qb test: status %d", resp.StatusCode)
	}
	return nil
}

func mapQBState(state string) string {
	switch state {
	case "downloading", "metaDL", "stalledDL":
		return "downloading"
	case "seeding", "uploading", "stalledUP":
		return "seeding"
	case "pausedDL", "pausedUP":
		return "paused"
	case "error", "missingFiles":
		return "error"
	default:
		return state
	}
}
