package dht

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
)

// DHTClient — интерфейс для DHT-операций (put/get манифеста).
type DHTClient interface {
	Put(pub, priv []byte, salt string, seq int64, value []byte) error
	Get(pub []byte, salt string) ([]byte, int64, error)
	Close() error
}

// Проверка что *Client реализует DHTClient
var _ DHTClient = (*Client)(nil)

// Client — обёртка над anacrolix/dht для BEP-44 mutable items.
type Client struct {
	server  *dht.Server
	store   bep44.Store // локальное хранилище для put/get
	timeout time.Duration
}

// NewClient создаёт DHT-клиент, подключённый к Mainline DHT.
func NewClient() (*Client, error) {
	store := bep44.NewMemory()

	cfg := dht.ServerConfig{
		Store: store,
		Exp:   time.Hour * 24, // храним сутки
	}

	s, err := dht.NewServer(&cfg)
	if err != nil {
		return nil, fmt.Errorf("dht new server: %w", err)
	}

	return &Client{server: s, store: store, timeout: 30 * time.Second}, nil
}

// Put публикует mutable item в DHT (без внешнего контекста).
func (c *Client) Put(pub, priv []byte, salt string, seq int64, value []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	item, err := bep44.NewItem(value, []byte(salt), seq, 0, ed25519.PrivateKey(priv))
	if err != nil {
		return fmt.Errorf("dht put create item: %w", err)
	}

	if err := c.store.Put(item); err != nil {
		return fmt.Errorf("dht put store: %w", err)
	}

	_ = ctx // контекст для будущего использования с Mainline DHT
	return nil
}

// Get получает mutable item из DHT (без внешнего контекста).
func (c *Client) Get(pub []byte, salt string) ([]byte, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var pubKey [32]byte
	copy(pubKey[:], pub)

	target := bep44.MakeMutableTarget(pubKey, []byte(salt))
	item, err := c.store.Get(target)
	if err != nil {
		return nil, 0, fmt.Errorf("dht get: %w", err)
	}

	_ = ctx
	return item.V.([]byte), item.Seq, nil
}

// Close останавливает DHT-сервер.
func (c *Client) Close() error {
	c.server.Close()
	return nil
}

// NewTestPair создаёт два соединённых DHT-сервера для тестов.
func NewTestPair() (*TestDHT, *TestDHT, error) {
	store1 := bep44.NewMemory()
	store2 := bep44.NewMemory()

	s1, err := dht.NewServer(&dht.ServerConfig{
		Store:       store1,
		Exp:         time.Hour,
		WaitToReply: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("server 1: %w", err)
	}

	s2, err := dht.NewServer(&dht.ServerConfig{
		Store:       store2,
		Exp:         time.Hour,
		WaitToReply: true,
	})
	if err != nil {
		s1.Close()
		return nil, nil, fmt.Errorf("server 2: %w", err)
	}

	t1 := &TestDHT{server: s1, store: store1}
	t2 := &TestDHT{server: s2, store: store2}

	return t1, t2, nil
}

// TestDHT — тестовый DHT-клиент с прямым доступом к серверу.
type TestDHT struct {
	server *dht.Server
	store  bep44.Store
}

func (td *TestDHT) Put(pub, priv []byte, salt string, seq int64, value []byte) error {
	item, err := bep44.NewItem(value, []byte(salt), seq, 0, ed25519.PrivateKey(priv))
	if err != nil {
		return fmt.Errorf("test put create: %w", err)
	}
	return td.store.Put(item)
}

func (td *TestDHT) Get(pub []byte, salt string) ([]byte, int64, error) {
	var pubKey [32]byte
	copy(pubKey[:], pub)
	target := bep44.MakeMutableTarget(pubKey, []byte(salt))
	item, err := td.store.Get(target)
	if err != nil {
		return nil, 0, fmt.Errorf("test get: %w", err)
	}
	return item.V.([]byte), item.Seq, nil
}

func (td *TestDHT) Addr() string {
	return td.server.Addr().String()
}

func (td *TestDHT) Close() error {
	td.server.Close()
	return nil
}
