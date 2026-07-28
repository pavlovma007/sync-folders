package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ————— Режимы работы IPFS —————

type ipfsMode int

const (
	modeMFS    ipfsMode = iota // Mutable File System (общий узел)
	modePubSub                 // PubSub (децентрализованный)
	modeHybrid                 // Гибрид: discovery через HTTP/SSH, данные через IPFS
)

// IPFSClient — транспорт для IPFS.
//
// Три режима работы (выбираются автоматически по полям конфига):
//
//  1. MFS (по умолчанию) — общий IPFS-узел, файлы в MFS:
//     api, mfs_root
//
//  2. PubSub — децентрализованный, обмен CID через PubSub:
//     api, pubsub_topic
//
//  3. Гибрид — discovery через HTTP/SSH, данные через IPFS:
//     api, discover_url, project
type IPFSClient struct {
	apiURL      string
	mfsRoot     string
	pubsubTopic string
	discoverURL string
	project     string
	pin         bool
	mode        ipfsMode

	client *http.Client

	// Для PubSub/Hybrid: кеш локально полученных файлов
	mu         sync.Mutex
	localFiles map[string]string // path → локальный путь
	lastCID    string            // последний полученный CID
}

// ————— API ответы IPFS —————

type filesLsResponse struct {
	Entries []filesLsEntry `json:"Entries"`
}

type filesLsEntry struct {
	Name string `json:"Name"`
	Type int    `json:"Type"` // 0=file, 1=dir
	Size int64  `json:"Size"`
	Hash string `json:"Hash"`
}

type filesStatResponse struct {
	Hash string `json:"Hash"`
	Size int64  `json:"Size"`
	Type string `json:"Type"`
}

type addResponse struct {
	Name string `json:"Name"`
	Hash string `json:"Hash"`
	Size string `json:"Size"`
}

// ————— Конструктор —————

func NewIPFSClient(cfg map[string]string) (*IPFSClient, error) {
	apiURL := cfg["api"]
	if apiURL == "" {
		apiURL = "http://127.0.0.1:5001"
	}

	c := &IPFSClient{
		apiURL:      strings.TrimRight(apiURL, "/"),
		mfsRoot:     cfg["mfs_root"],
		pubsubTopic: cfg["pubsub_topic"],
		discoverURL: cfg["discover_url"],
		project:     cfg["project"],
		pin:         true,
		client:      &http.Client{Timeout: 60 * time.Second},
		localFiles:  make(map[string]string),
	}

	// Определяем режим работы
	switch {
	case c.discoverURL != "":
		c.mode = modeHybrid
		if c.mfsRoot == "" {
			c.mfsRoot = "/sync"
		}
	case c.pubsubTopic != "":
		c.mode = modePubSub
		if c.mfsRoot == "" {
			c.mfsRoot = "/sync"
		}
	default:
		c.mode = modeMFS
		if c.mfsRoot == "" {
			c.mfsRoot = "/sync"
		}
	}

	if cfg["pin"] == "false" {
		c.pin = false
	}

	return c, nil
}

func (ic *IPFSClient) Name() string { return "ipfs" }

// ————— HTTP helpers —————

func (ic *IPFSClient) apiPost(endpoint string, body io.Reader, contentType string) (*http.Response, error) {
	url := ic.apiURL + endpoint
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return ic.client.Do(req)
}

func (ic *IPFSClient) apiPostForm(endpoint string, params map[string]string) (*http.Response, error) {
	url := ic.apiURL + endpoint + "?" + encodeParams(params)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	return ic.client.Do(req)
}

func encodeParams(params map[string]string) string {
	var parts []string
	for k, v := range params {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "&")
}

// ————— MFS methods —————

func (ic *IPFSClient) mfsPath(remotePath string) string {
	return path.Join(ic.mfsRoot, remotePath)
}

func (ic *IPFSClient) filesWrite(mfsPath string, data []byte) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", path.Base(mfsPath))
	if err != nil {
		return fmt.Errorf("ipfs write form: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("ipfs write copy: %w", err)
	}
	w.Close()

	params := fmt.Sprintf("arg=%s&create=true&parents=true", mfsPath)
	resp, err := ic.apiPost("/api/v0/files/write?"+params, &buf, w.FormDataContentType())
	if err != nil {
		return fmt.Errorf("ipfs write: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ipfs write: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (ic *IPFSClient) filesRead(mfsPath string) ([]byte, error) {
	resp, err := ic.apiPostForm("/api/v0/files/read", map[string]string{"arg": mfsPath})
	if err != nil {
		return nil, fmt.Errorf("ipfs read: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ipfs read: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func (ic *IPFSClient) filesLs(mfsPath string) ([]filesLsEntry, error) {
	resp, err := ic.apiPostForm("/api/v0/files/ls", map[string]string{"arg": mfsPath})
	if err != nil {
		return nil, fmt.Errorf("ipfs ls: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ipfs ls: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var lsResp filesLsResponse
	if err := json.NewDecoder(resp.Body).Decode(&lsResp); err != nil {
		return nil, fmt.Errorf("ipfs ls parse: %w", err)
	}
	return lsResp.Entries, nil
}

func (ic *IPFSClient) filesRm(mfsPath string) error {
	resp, err := ic.apiPostForm("/api/v0/files/rm", map[string]string{"arg": mfsPath})
	if err != nil {
		return fmt.Errorf("ipfs rm: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ipfs rm: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (ic *IPFSClient) filesMkdir(mfsPath string) error {
	resp, err := ic.apiPostForm("/api/v0/files/mkdir", map[string]string{
		"arg":     mfsPath,
		"parents": "true",
	})
	if err != nil {
		return fmt.Errorf("ipfs mkdir: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// ————— IPFS add (файл → CID) —————

func (ic *IPFSClient) ipfsAdd(data []byte, fileName string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return "", fmt.Errorf("ipfs add form: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("ipfs add copy: %w", err)
	}
	w.Close()

	resp, err := ic.apiPost("/api/v0/add?pin=false", &buf, w.FormDataContentType())
	if err != nil {
		return "", fmt.Errorf("ipfs add: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ipfs add: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var addResp addResponse
	if err := json.NewDecoder(resp.Body).Decode(&addResp); err != nil {
		return "", fmt.Errorf("ipfs add parse: %w", err)
	}
	return addResp.Hash, nil
}

// ————— IPFS get (CID → файл) —————

func (ic *IPFSClient) ipfsGet(cid, destPath string) error {
	resp, err := ic.apiPostForm("/api/v0/get", map[string]string{"arg": cid})
	if err != nil {
		return fmt.Errorf("ipfs get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ipfs get: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// IPFS возвращает tar-архив. Для одного файла — просто сохраняем тело.
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("ipfs get mkdir: %w", err)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("ipfs get create: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("ipfs get copy: %w", err)
	}
	return nil
}

// ————— Pin —————

func (ic *IPFSClient) pinAdd(cid string) error {
	if !ic.pin {
		return nil
	}
	resp, err := ic.apiPostForm("/api/v0/pin/add", map[string]string{"arg": cid})
	if err != nil {
		return fmt.Errorf("ipfs pin: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("ipfs pin: status %d", resp.StatusCode)
	}
	return nil
}

func (ic *IPFSClient) pinRm(cid string) error {
	resp, err := ic.apiPostForm("/api/v0/pin/rm", map[string]string{"arg": cid})
	if err != nil {
		return fmt.Errorf("ipfs pin rm: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// ————— PubSub —————

// pubSubPublish публикует сообщение в PubSub-канал.
// POST body = message (base64 или raw), arg = topic в URL.
func (ic *IPFSClient) pubSubPublish(topic, message string) error {
	url := ic.apiURL + "/api/v0/pubsub/pub?arg=" + topic
	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte(message)))
	if err != nil {
		return fmt.Errorf("ipfs pubsub pub req: %w", err)
	}
	resp, err := ic.client.Do(req)
	if err != nil {
		return fmt.Errorf("ipfs pubsub pub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ipfs pubsub pub: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// pubSubSubscribe подписывается на PubSub-канал и возвращает канал с сообщениями.
func (ic *IPFSClient) pubSubSubscribe(topic string) (<-chan string, error) {
	msgCh := make(chan string, 100)

	url := ic.apiURL + "/api/v0/pubsub/sub?arg=" + topic
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ipfs pubsub sub req: %w", err)
	}

	resp, err := ic.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ipfs pubsub sub: %w", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("ipfs pubsub sub: status %d", resp.StatusCode)
	}

	// Читаем NDJSON-поток из long-lived соединения
	go func() {
		defer resp.Body.Close()
		decoder := json.NewDecoder(resp.Body)
		for {
			var msg struct {
				From     string   `json:"from"`
				Data     string   `json:"data"`
				Seqno    string   `json:"seqno"`
				TopicIDs []string `json:"topicIDs"`
			}
			if err := decoder.Decode(&msg); err != nil {
				close(msgCh)
				return
			}
			// Data — base64-encoded
			if msg.Data != "" {
				msgCh <- msg.Data
			}
		}
	}()

	return msgCh, nil
}

// ————— Hybrid discovery —————

type cidMessage struct {
	CID    string `json:"cid"`
	Author string `json:"author"`
	Time   int64  `json:"time"`
}

// hybridPublish отправляет CID на discovery-сервер.
func (ic *IPFSClient) hybridPublish(cid string) error {
	if ic.discoverURL == "" {
		return nil
	}
	msg := cidMessage{
		CID:    cid,
		Author: "sync-folders",
		Time:   time.Now().Unix(),
	}
	body, _ := json.Marshal(msg)

	url := ic.discoverURL
	if ic.project != "" {
		url += "?project=" + ic.project
	}

	resp, err := ic.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("hybrid publish: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("hybrid publish: status %d", resp.StatusCode)
	}
	return nil
}

// hybridFetch получает последний CID с discovery-сервера.
func (ic *IPFSClient) hybridFetch() (string, error) {
	if ic.discoverURL == "" {
		return "", fmt.Errorf("hybrid: discover_url not configured")
	}
	url := ic.discoverURL + "/latest"
	if ic.project != "" {
		url += "?project=" + ic.project
	}

	resp, err := ic.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("hybrid fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("hybrid fetch: status %d", resp.StatusCode)
	}

	var msg cidMessage
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return "", fmt.Errorf("hybrid fetch parse: %w", err)
	}
	if msg.CID == "" {
		return "", fmt.Errorf("hybrid fetch: empty CID")
	}
	return msg.CID, nil
}

// ————— Transport: List —————

func (ic *IPFSClient) List(remotePath string) ([]FileInfo, error) {
	switch ic.mode {
	case modeMFS:
		return ic.listMFS(remotePath)
	case modePubSub:
		return ic.listLocal()
	case modeHybrid:
		return ic.listLocal()
	default:
		return ic.listMFS(remotePath)
	}
}

func (ic *IPFSClient) listMFS(remotePath string) ([]FileInfo, error) {
	listPath := ic.mfsPath(remotePath)
	entries, err := ic.filesLs(listPath)
	if err != nil {
		return nil, fmt.Errorf("ipfs list: %w", err)
	}
	var result []FileInfo
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		relPath := e.Name
		if remotePath != "" {
			relPath = remotePath + "/" + e.Name
		}
		result = append(result, FileInfo{
			Name:  e.Name,
			Path:  relPath,
			Size:  e.Size,
			IsDir: e.Type == 1,
		})
	}
	return result, nil
}

func (ic *IPFSClient) listLocal() ([]FileInfo, error) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	var result []FileInfo
	for path, _ := range ic.localFiles {
		result = append(result, FileInfo{
			Name: filepath.Base(path),
			Path: path,
			Size: 0,
		})
	}
	return result, nil
}

// ————— Transport: Push —————

func (ic *IPFSClient) Push(localPath, remotePath string) error {
	switch ic.mode {
	case modeMFS:
		return ic.pushMFS(localPath, remotePath)
	case modePubSub:
		return ic.pushPubSub(localPath, remotePath)
	case modeHybrid:
		return ic.pushHybrid(localPath, remotePath)
	default:
		return ic.pushMFS(localPath, remotePath)
	}
}

func (ic *IPFSClient) pushMFS(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("ipfs push read: %w", err)
	}
	destPath := ic.mfsPath(remotePath)
	parentDir := path.Dir(destPath)
	if parentDir != "." && parentDir != "/" {
		_ = ic.filesMkdir(parentDir)
	}
	return ic.filesWrite(destPath, data)
}

func (ic *IPFSClient) pushPubSub(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("ipfs push read: %w", err)
	}

	// Добавляем файл в IPFS → получаем CID
	fileName := filepath.Base(localPath)
	cid, err := ic.ipfsAdd(data, fileName)
	if err != nil {
		return fmt.Errorf("ipfs push add: %w", err)
	}

	// Закрепляем
	if err := ic.pinAdd(cid); err != nil {
		return fmt.Errorf("ipfs push pin: %w", err)
	}

	// Публикуем CID в PubSub
	if ic.pubsubTopic != "" {
		msg := cidMessage{
			CID:    cid,
			Author: "sync-folders",
			Time:   time.Now().Unix(),
		}
		msgJSON, _ := json.Marshal(msg)
		if err := ic.pubSubPublish(ic.pubsubTopic, string(msgJSON)); err != nil {
			return fmt.Errorf("ipfs push publish: %w", err)
		}
	}

	// Сохраняем в локальном кеше
	ic.mu.Lock()
	ic.localFiles[remotePath] = localPath
	ic.lastCID = cid
	ic.mu.Unlock()

	return nil
}

func (ic *IPFSClient) pushHybrid(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("ipfs push read: %w", err)
	}

	fileName := filepath.Base(localPath)
	cid, err := ic.ipfsAdd(data, fileName)
	if err != nil {
		return fmt.Errorf("ipfs push add: %w", err)
	}

	if err := ic.pinAdd(cid); err != nil {
		return fmt.Errorf("ipfs push pin: %w", err)
	}

	// Отправляем CID на discovery-сервер
	if err := ic.hybridPublish(cid); err != nil {
		return fmt.Errorf("ipfs push hybrid: %w", err)
	}

	ic.mu.Lock()
	ic.localFiles[remotePath] = localPath
	ic.lastCID = cid
	ic.mu.Unlock()

	return nil
}

// ————— Transport: Pull —————

func (ic *IPFSClient) Pull(remotePath, localPath string) error {
	switch ic.mode {
	case modeMFS:
		return ic.pullMFS(remotePath, localPath)
	case modePubSub:
		return ic.pullPubSub(remotePath, localPath)
	case modeHybrid:
		return ic.pullHybrid(remotePath, localPath)
	default:
		return ic.pullMFS(remotePath, localPath)
	}
}

func (ic *IPFSClient) pullMFS(remotePath, localPath string) error {
	sourcePath := ic.mfsPath(remotePath)
	data, err := ic.filesRead(sourcePath)
	if err != nil {
		return fmt.Errorf("ipfs pull: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("ipfs pull mkdir: %w", err)
	}
	return os.WriteFile(localPath, data, 0644)
}

func (ic *IPFSClient) pullPubSub(remotePath, localPath string) error {
	// В PubSub режиме remotePath — это CID
	cid := remotePath
	if err := ic.ipfsGet(cid, localPath); err != nil {
		return fmt.Errorf("ipfs pull pubsub: %w", err)
	}
	if err := ic.pinAdd(cid); err != nil {
		return fmt.Errorf("ipfs pull pin: %w", err)
	}
	ic.mu.Lock()
	ic.localFiles[localPath] = localPath
	ic.lastCID = cid
	ic.mu.Unlock()
	return nil
}

func (ic *IPFSClient) pullHybrid(remotePath, localPath string) error {
	// В Hybrid режиме remotePath игнорируется, тянем последний CID
	cid, err := ic.hybridFetch()
	if err != nil {
		return fmt.Errorf("ipfs pull hybrid fetch: %w", err)
	}
	if err := ic.ipfsGet(cid, localPath); err != nil {
		return fmt.Errorf("ipfs pull hybrid get: %w", err)
	}
	if err := ic.pinAdd(cid); err != nil {
		return fmt.Errorf("ipfs pull pin: %w", err)
	}
	ic.mu.Lock()
	ic.localFiles[localPath] = localPath
	ic.lastCID = cid
	ic.mu.Unlock()
	return nil
}

// ————— Transport: Delete —————

func (ic *IPFSClient) Delete(remotePath string) error {
	switch ic.mode {
	case modeMFS:
		return ic.deleteMFS(remotePath)
	case modePubSub:
		return ic.deletePubSub(remotePath)
	case modeHybrid:
		return ic.deletePubSub(remotePath)
	default:
		return ic.deleteMFS(remotePath)
	}
}

func (ic *IPFSClient) deleteMFS(remotePath string) error {
	delPath := ic.mfsPath(remotePath)
	return ic.filesRm(delPath)
}

func (ic *IPFSClient) deletePubSub(remotePath string) error {
	// В PubSub/Hybrid удаляем из локального кеша
	ic.mu.Lock()
	delete(ic.localFiles, remotePath)
	ic.mu.Unlock()
	return nil
}

// ————— Transport: Test —————

func (ic *IPFSClient) Test() error {
	resp, err := ic.apiPostForm("/api/v0/files/stat", map[string]string{"arg": "/"})
	if err != nil {
		return fmt.Errorf("ipfs test: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("ipfs test: status %d", resp.StatusCode)
	}
	return nil
}

// ————— PubSub Subscribe (для внешнего вызова) —————

// Subscribe возвращает канал с CID-сообщениями из PubSub.
// Используется в daemon-режиме.
func (ic *IPFSClient) Subscribe() (<-chan string, error) {
	if ic.pubsubTopic == "" {
		return nil, fmt.Errorf("ipfs: pubsub_topic not configured")
	}
	return ic.pubSubSubscribe(ic.pubsubTopic)
}

// LastCID возвращает последний полученный CID.
func (ic *IPFSClient) LastCID() string {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.lastCID
}
