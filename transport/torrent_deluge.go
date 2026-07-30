package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DelugeClient — клиент для Deluge Web API (JSON-RPC).
type DelugeClient struct {
	apiURL   string
	password string
	client   *http.Client
}

// NewDelugeClient создаёт клиент для Deluge.
func NewDelugeClient(apiURL, password string) *DelugeClient {
	return &DelugeClient{
		apiURL:   strings.TrimRight(apiURL, "/"),
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DelugeClient) Name() string { return "deluge" }

// delugeRPCRequest — универсальный JSON-RPC запрос.
type delugeRPCRequest struct {
	Method string      `json:"method"`
	Params []any       `json:"params"`
	ID     int         `json:"id"`
}

// delugeRPCResponse — ответ от Deluge.
type delugeRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
	ID int `json:"id"`
}

func (d *DelugeClient) rpcCall(method string, params []any) (json.RawMessage, error) {
	// Login if password is set
	if d.password != "" {
		loginReq := delugeRPCRequest{
			Method: "auth.login",
			Params: []any{d.password},
			ID:     1,
		}
		_, err := d.doRPC(loginReq)
		if err != nil {
			return nil, fmt.Errorf("deluge login: %w", err)
		}
	}

	req := delugeRPCRequest{
		Method: method,
		Params: params,
		ID:     2,
	}

	return d.doRPC(req)
}

func (d *DelugeClient) doRPC(req delugeRPCRequest) (json.RawMessage, error) {
	body, _ := json.Marshal(req)
	resp, err := d.client.Post(d.apiURL+"/json", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("deluge rpc: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deluge rpc read: %w", err)
	}

	var rpcResp delugeRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("deluge rpc decode: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("deluge rpc error: %s (code=%d)", rpcResp.Error.Message, rpcResp.Error.Code)
	}

	return rpcResp.Result, nil
}

func (d *DelugeClient) AddMagnet(magnetURI, savePath string) (string, error) {
	options := map[string]any{}
	if savePath != "" {
		options["download_location"] = savePath
	}

	_, err := d.rpcCall("core.add_torrent_url", []any{magnetURI, options})
	if err != nil {
		return "", fmt.Errorf("deluge add magnet: %w", err)
	}
	return "", nil
}

func (d *DelugeClient) AddTorrentFile(data []byte, savePath string) (string, error) {
	options := map[string]any{}
	if savePath != "" {
		options["download_location"] = savePath
	}

	_, err := d.rpcCall("core.add_torrent_file", []any{"snapshot.torrent", data, options})
	if err != nil {
		return "", fmt.Errorf("deluge add file: %w", err)
	}
	return "", nil
}

func (d *DelugeClient) List() ([]TorrentInfo, error) {
	// Get all torrents status with relevant fields
	fields := []string{"hash", "name", "progress", "state", "download_location", "total_size"}
	statusMapRaw, err := d.rpcCall("core.get_torrents_status", []any{map[string]any{}, fields})
	if err != nil {
		return nil, fmt.Errorf("deluge list: %w", err)
	}

	var statusMap map[string]json.RawMessage
	if err := json.Unmarshal(statusMapRaw, &statusMap); err != nil {
		return nil, fmt.Errorf("deluge list decode: %w", err)
	}

	result := make([]TorrentInfo, 0, len(statusMap))
	for _, raw := range statusMap {
		var st struct {
			Hash       string  `json:"hash"`
			Name       string  `json:"name"`
			Progress   float64 `json:"progress"`
			State      string  `json:"state"`
			SavePath   string  `json:"download_location"`
			TotalSize  int64   `json:"total_size"`
		}
		if err := json.Unmarshal(raw, &st); err != nil {
			continue
		}
		result = append(result, TorrentInfo{
			Hash:     st.Hash,
			Name:     st.Name,
			Progress: st.Progress,
			State:    mapDelugeState(st.State),
			SavePath: st.SavePath,
			Size:     st.TotalSize,
		})
	}
	return result, nil
}

func (d *DelugeClient) GetInfo(hash string) (TorrentInfo, error) {
	fields := []string{"hash", "name", "progress", "state", "download_location", "total_size"}
	raw, err := d.rpcCall("core.get_torrents_status", []any{map[string]any{"hash": hash}, fields})
	if err != nil {
		return TorrentInfo{}, fmt.Errorf("deluge info: %w", err)
	}

	var statusMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &statusMap); err != nil {
		return TorrentInfo{}, fmt.Errorf("deluge info decode: %w", err)
	}

	for _, rawItem := range statusMap {
		var st struct {
			Hash       string  `json:"hash"`
			Name       string  `json:"name"`
			Progress   float64 `json:"progress"`
			State      string  `json:"state"`
			SavePath   string  `json:"download_location"`
			TotalSize  int64   `json:"total_size"`
		}
		if err := json.Unmarshal(rawItem, &st); err != nil {
			continue
		}
		if st.Hash == hash {
			return TorrentInfo{
				Hash:     st.Hash,
				Name:     st.Name,
				Progress: st.Progress,
				State:    mapDelugeState(st.State),
				SavePath: st.SavePath,
				Size:     st.TotalSize,
			}, nil
		}
	}
	return TorrentInfo{}, fmt.Errorf("deluge: torrent %q not found", hash)
}

func (d *DelugeClient) Delete(hash string, deleteFiles bool) error {
	_, err := d.rpcCall("core.remove_torrent", []any{hash, deleteFiles})
	if err != nil {
		return fmt.Errorf("deluge delete: %w", err)
	}
	return nil
}

func (d *DelugeClient) Test() error {
	// Try getting session state as a health check
	_, err := d.rpcCall("daemon.get_method_list", []any{})
	if err != nil {
		return fmt.Errorf("deluge test: %w", err)
	}
	return nil
}

func mapDelugeState(state string) string {
	switch state {
	case "Downloading", "DownloadingMetadata":
		return "downloading"
	case "Seeding", "Uploading":
		return "seeding"
	case "Paused":
		return "paused"
	case "Error":
		return "error"
	default:
		return state
	}
}
