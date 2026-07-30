package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TransmissionClient — клиент для Transmission RPC.
type TransmissionClient struct {
	apiURL    string
	username  string
	password  string
	client    *http.Client
	mu        sync.Mutex
	sessionID string
}

// NewTransmissionClient создаёт клиент для Transmission.
func NewTransmissionClient(apiURL, username, password string) *TransmissionClient {
	return &TransmissionClient{
		apiURL:   strings.TrimRight(apiURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *TransmissionClient) Name() string { return "transmission" }

type transmissionRequest struct {
	Method    string `json:"method"`
	Arguments any    `json:"arguments,omitempty"`
	Tag       int    `json:"tag,omitempty"`
}

type transmissionResponse struct {
	Result string          `json:"result"`
	Args   json.RawMessage `json:"arguments"`
	Tag    int             `json:"tag,omitempty"`
}

func (t *TransmissionClient) rpc(method string, args any) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	req := transmissionRequest{
		Method:    method,
		Arguments: args,
	}

	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST", t.apiURL+"/transmission/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("transmission req: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if t.username != "" || t.password != "" {
		httpReq.SetBasicAuth(t.username, t.password)
	}
	if t.sessionID != "" {
		httpReq.Header.Set("X-Transmission-Session-Id", t.sessionID)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("transmission rpc: %w", err)
	}
	defer resp.Body.Close()

	// Handle session-id challenge (first request)
	if resp.StatusCode == 409 {
		t.sessionID = resp.Header.Get("X-Transmission-Session-Id")
		if t.sessionID == "" {
			return nil, fmt.Errorf("transmission: 409 without session-id header")
		}
		// Retry with session ID
		httpReq.Header.Set("X-Transmission-Session-Id", t.sessionID)
		resp, err = t.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("transmission rpc retry: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("transmission: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("transmission read: %w", err)
	}

	var tr transmissionResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return nil, fmt.Errorf("transmission decode: %w", err)
	}
	if tr.Result != "success" {
		return nil, fmt.Errorf("transmission: %s", tr.Result)
	}

	return tr.Args, nil
}

func (t *TransmissionClient) AddMagnet(magnetURI, savePath string) (string, error) {
	args := map[string]any{
		"filename": magnetURI,
	}
	if savePath != "" {
		args["download-dir"] = savePath
	}

	raw, err := t.rpc("torrent-add", args)
	if err != nil {
		return "", fmt.Errorf("transmission add magnet: %w", err)
	}

	var result struct {
		TorrentAdded *struct {
			HashString string `json:"hashString"`
			ID         int    `json:"id"`
			Name       string `json:"name"`
		} `json:"torrent-added"`
		TorrentDuplicate *struct {
			HashString string `json:"hashString"`
			ID         int    `json:"id"`
		} `json:"torrent-duplicate"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", nil
	}

	if result.TorrentAdded != nil {
		return result.TorrentAdded.HashString, nil
	}
	if result.TorrentDuplicate != nil {
		return result.TorrentDuplicate.HashString, nil
	}
	return "", nil
}

func (t *TransmissionClient) AddTorrentFile(data []byte, savePath string) (string, error) {
	args := map[string]any{
		"metainfo": data,
	}
	if savePath != "" {
		args["download-dir"] = savePath
	}

	raw, err := t.rpc("torrent-add", args)
	if err != nil {
		return "", fmt.Errorf("transmission add file: %w", err)
	}

	var result struct {
		TorrentAdded *struct {
			HashString string `json:"hashString"`
		} `json:"torrent-added"`
		TorrentDuplicate *struct {
			HashString string `json:"hashString"`
		} `json:"torrent-duplicate"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", nil
	}

	if result.TorrentAdded != nil {
		return result.TorrentAdded.HashString, nil
	}
	if result.TorrentDuplicate != nil {
		return result.TorrentDuplicate.HashString, nil
	}
	return "", nil
}

func (t *TransmissionClient) List() ([]TorrentInfo, error) {
	raw, err := t.rpc("torrent-get", map[string]any{
		"fields": []string{"id", "hashString", "name", "percentDone", "status", "downloadDir", "totalSize"},
	})
	if err != nil {
		return nil, fmt.Errorf("transmission list: %w", err)
	}

	var result struct {
		Torrents []struct {
			ID           int     `json:"id"`
			HashString   string  `json:"hashString"`
			Name         string  `json:"name"`
			PercentDone  float64 `json:"percentDone"`
			Status       int     `json:"status"`
			DownloadDir  string  `json:"downloadDir"`
			TotalSize    int64   `json:"totalSize"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("transmission list decode: %w", err)
	}

	list := make([]TorrentInfo, 0, len(result.Torrents))
	for _, t := range result.Torrents {
		list = append(list, TorrentInfo{
			Hash:     t.HashString,
			Name:     t.Name,
			Progress: t.PercentDone,
			State:    mapTransmissionStatus(t.Status),
			SavePath: t.DownloadDir,
			Size:     t.TotalSize,
		})
	}
	return list, nil
}

func (t *TransmissionClient) GetInfo(hash string) (TorrentInfo, error) {
	raw, err := t.rpc("torrent-get", map[string]any{
		"fields": []string{"id", "hashString", "name", "percentDone", "status", "downloadDir", "totalSize"},
	})
	if err != nil {
		return TorrentInfo{}, fmt.Errorf("transmission info: %w", err)
	}

	var result struct {
		Torrents []struct {
			ID           int     `json:"id"`
			HashString   string  `json:"hashString"`
			Name         string  `json:"name"`
			PercentDone  float64 `json:"percentDone"`
			Status       int     `json:"status"`
			DownloadDir  string  `json:"downloadDir"`
			TotalSize    int64   `json:"totalSize"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return TorrentInfo{}, fmt.Errorf("transmission info decode: %w", err)
	}

	for _, t := range result.Torrents {
		if t.HashString == hash {
			return TorrentInfo{
				Hash:     t.HashString,
				Name:     t.Name,
				Progress: t.PercentDone,
				State:    mapTransmissionStatus(t.Status),
				SavePath: t.DownloadDir,
				Size:     t.TotalSize,
			}, nil
		}
	}
	return TorrentInfo{}, fmt.Errorf("transmission: torrent %q not found", hash)
}

func (t *TransmissionClient) Delete(hash string, deleteFiles bool) error {
	_, err := t.rpc("torrent-remove", map[string]any{
		"ids":             []string{hash},
		"delete-local-data": deleteFiles,
	})
	if err != nil {
		return fmt.Errorf("transmission delete: %w", err)
	}
	return nil
}

func (t *TransmissionClient) Test() error {
	_, err := t.rpc("session-get", map[string]any{})
	if err != nil {
		return fmt.Errorf("transmission test: %w", err)
	}
	return nil
}

// Transmission status codes:
// 0 = stopped, 1 = check pending, 2 = checking, 3 = download pending,
// 4 = downloading, 5 = seed pending, 6 = seeding
func mapTransmissionStatus(status int) string {
	switch status {
	case 0:
		return "paused"
	case 1, 2:
		return "downloading"
	case 3, 4:
		return "downloading"
	case 5, 6:
		return "seeding"
	default:
		return "error"
	}
}
