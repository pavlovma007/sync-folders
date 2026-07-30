package transport

import (
	"fmt"
	"sync"

	"sync-folders/dht"
)

// Проверка что MockDHTClient реализует dht.DHTClient
var _ dht.DHTClient = (*MockDHTClient)(nil)

// MockTorrentClient — мок торрент-клиента для тестов.
type MockTorrentClient struct {
	mu       sync.Mutex
	torrents map[string]TorrentInfo // hash → info
}

func NewMockTorrentClient() *MockTorrentClient {
	return &MockTorrentClient{torrents: make(map[string]TorrentInfo)}
}

func (m *MockTorrentClient) Name() string { return "mock" }

func (m *MockTorrentClient) AddMagnet(magnetURI, savePath string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := fmt.Sprintf("hash-%d", len(m.torrents))
	m.torrents[hash] = TorrentInfo{
		Hash:     hash,
		Name:     magnetURI,
		Progress: 1.0,
		State:    "seeding",
		SavePath: savePath,
	}
	return hash, nil
}

func (m *MockTorrentClient) AddTorrentFile(data []byte, savePath string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := fmt.Sprintf("hash-%d", len(m.torrents))
	m.torrents[hash] = TorrentInfo{
		Hash:     hash,
		Name:     "snapshot.torrent",
		Progress: 1.0,
		State:    "seeding",
		SavePath: savePath,
	}
	return hash, nil
}

func (m *MockTorrentClient) GetInfo(hash string) (TorrentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.torrents[hash]
	if !ok {
		return TorrentInfo{}, fmt.Errorf("torrent %q not found", hash)
	}
	return info, nil
}

func (m *MockTorrentClient) List() ([]TorrentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []TorrentInfo
	for _, info := range m.torrents {
		list = append(list, info)
	}
	return list, nil
}

func (m *MockTorrentClient) Delete(hash string, deleteFiles bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.torrents, hash)
	return nil
}

func (m *MockTorrentClient) Test() error { return nil }

// MockDHTClient — мок DHT-клиента для тестов TorrentTransport.
type MockDHTClient struct {
	mu    sync.Mutex
	items map[string]DHTItem // target → item
}

type DHTItem struct {
	Value []byte
	Seq   int64
}

func NewMockDHTClient() *MockDHTClient {
	return &MockDHTClient{items: make(map[string]DHTItem)}
}

func (m *MockDHTClient) Put(pub, priv []byte, salt string, seq int64, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := string(pub) + ":" + salt
	m.items[key] = DHTItem{Value: value, Seq: seq}
	return nil
}

func (m *MockDHTClient) Get(pub []byte, salt string) ([]byte, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := string(pub) + ":" + salt
	item, ok := m.items[key]
	if !ok {
		return nil, 0, fmt.Errorf("dht item not found")
	}
	return item.Value, item.Seq, nil
}

func (m *MockDHTClient) Close() error { return nil }
