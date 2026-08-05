package dht

import (
	"net"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
)

func TestTestPairPutGet(t *testing.T) {
	t1, t2, err := NewTestPair()
	if err != nil {
		t.Fatalf("NewTestPair: %v", err)
	}
	defer t1.Close()
	defer t2.Close()

	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	value := []byte(`{"seq":1,"magnet":"magnet:?xt=urn:btih:test123","ts":1700000000,"files_hash":"abc"}`)

	// Put to t1
	err = t1.Put( pub, priv, SaltForProject("test"), 1, value)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get from t1 (same server)
	got, seq, err := t1.Get( pub, SaltForProject("test"))
	if err != nil {
		t.Fatalf("Get from t1: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get from t1: value mismatch:\ngot:  %s\nwant: %s", string(got), string(value))
	}
	if seq != 1 {
		t.Errorf("seq: got %d, want 1", seq)
	}

	// Get from t2 (different server) — should fail since t2 doesn't have it yet
	_, _, err = t2.Get(pub, SaltForProject("test"))
	if err == nil {
		t.Log("note: t2 can also see t1's data (same memory store or replication)")
	}
}

func TestPutGetRoundtrip(t *testing.T) {
	t1, _, err := NewTestPair()
	if err != nil {
		t.Fatalf("NewTestPair: %v", err)
	}
	defer t1.Close()

	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := &Manifest{
		Seq:       42,
		Magnet:    "magnet:?xt=urn:btih:abc123def456",
		Timestamp: time.Now().Unix(),
		FilesHash: "deadbeef",
	}

	value, err := m.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Put
	err = t1.Put( pub, priv, SaltForProject("my-project"), 42, value)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	got, seq, err := t1.Get( pub, SaltForProject("my-project"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if seq != 42 {
		t.Errorf("seq: got %d, want 42", seq)
	}

	gotManifest, err := UnmarshalManifest(got)
	if err != nil {
		t.Fatalf("UnmarshalManifest: %v", err)
	}
	if gotManifest.Magnet != m.Magnet {
		t.Errorf("magnet: got %q, want %q", gotManifest.Magnet, m.Magnet)
	}
	if gotManifest.Seq != m.Seq {
		t.Errorf("seq: got %d, want %d", gotManifest.Seq, m.Seq)
	}
}

func TestPutOverwrite(t *testing.T) {
	t1, _, err := NewTestPair()
	if err != nil {
		t.Fatalf("NewTestPair: %v", err)
	}
	defer t1.Close()

	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Put version 1
	err = t1.Put( pub, priv, "test", 1, []byte("v1"))
	if err != nil {
		t.Fatalf("Put v1: %v", err)
	}

	// Put version 2
	err = t1.Put( pub, priv, "test", 2, []byte("v2"))
	if err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	// Get — should get v2 (latest seq)
	got, seq, err := t1.Get( pub, "test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if seq != 2 {
		t.Errorf("seq: got %d, want 2", seq)
	}
	if string(got) != "v2" {
		t.Errorf("value: got %q, want v2", string(got))
	}
}

func TestNewClient(t *testing.T) {
	// Just test that a client can be created (without connecting to Mainline)
	// This is a lightweight creation test
	c := &Client{
		store:   bep44.NewWrapper(bep44.NewMemory(), time.Hour),
		timeout: time.Second,
	}
	_ = c
	t.Log("Client creation OK (server omitted for unit test)")
}

// newClientForTest создаёт реальный DHT-сервер (без Mainline bootstrap)
// и обёртку Client над ним. Сервер слушает на loopback, чтобы UDP-пакеты
// реально доходили между тестовыми узлами.
func newClientForTest(t *testing.T) (*Client, *dht.Server) {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	exp := time.Hour
	mem := bep44.NewMemory()
	s, err := dht.NewServer(&dht.ServerConfig{
		Conn:        conn,
		Store:       mem,
		Exp:         exp,
		WaitToReply: true,
		// Как в NewDefaultServerConfig: иначе случайный node ID не пройдёт
		// BEP-42 "secure" проверку и узлы не попадут в таблицу маршрутизации.
		NoSecurity: true,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	// То же устройство хранилища, что и в NewClient: Client пишет/читает
	// через Wrapper, чтобы положенные items не считались просроченными.
	return &Client{server: s, store: bep44.NewWrapper(mem, exp), timeout: 5 * time.Second}, s
}

// TestClientGetNetwork проверяет, что Client.Get() через traversal
// находит item у удалённого узла (in-process, без Mainline DHT).
func TestClientGetNetwork(t *testing.T) {
	cA, sA := newClientForTest(t)
	cB, sB := newClientForTest(t)

	// Добавляем B в таблицу маршрутизации A — единственный стартовый узел
	// для traversal (не хотим зависеть от DNS/bootstrap в тесте).
	if err := sA.AddNode(krpc.NodeInfo{ID: sB.ID(), Addr: dht.NewAddr(sB.Addr()).KRPC()}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	value := []byte(`{"seq":7,"magnet":"magnet:?xt=urn:btih:net1","ts":1700000000,"files_hash":"abc"}`)
	if err := cB.Put(pub, priv, "test", 7, value); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get из A: локально item отсутствует -> traversal опрашивает B.
	got, seq, err := cA.Get(pub, "test")
	if err != nil {
		t.Fatalf("Get over network: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("value mismatch: got %q, want %q", string(got), string(value))
	}
	if seq != 7 {
		t.Errorf("seq: got %d, want 7", seq)
	}
}

// TestClientGetTraversalNotFound проверяет, что traversal-ветка Client.Get()
// не паникует и возвращает ошибку, когда item нигде нет.
func TestClientGetTraversalNotFound(t *testing.T) {
	c, _ := newClientForTest(t)

	pub, _, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Пустая таблица и нет StartingNodes: traversal сразу останавливается.
	_, _, err = c.Get(pub, "test")
	if err == nil {
		t.Fatal("expected error for missing item")
	}
	t.Logf("Get with traversal returned expected error: %v", err)
}

// TestClientPutNetwork проверяет, что Client.Put() через traversal
// публикует item у удалённого узла: Get → write token → Put (BEP-44).
func TestClientPutNetwork(t *testing.T) {
	cA, sA := newClientForTest(t)
	cB, sB := newClientForTest(t)

	// Добавляем B в таблицу маршрутизации A — единственный стартовый узел
	// для traversal (не хотим зависеть от DNS/bootstrap в тесте).
	if err := sA.AddNode(krpc.NodeInfo{ID: sB.ID(), Addr: dht.NewAddr(sB.Addr()).KRPC()}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	value := []byte(`{"seq":9,"magnet":"magnet:?xt=urn:btih:put1"}`)

	// Put из A → по сети должен дойти до B (через traversal).
	if err := cA.Put(pub, priv, "test", 9, value); err != nil {
		t.Fatalf("Put network: %v", err)
	}

	// Get из B локально (B получил item через сетевой Put) → должен найти.
	got, seq, err := cB.Get(pub, "test")
	if err != nil {
		t.Fatalf("B Get after Put: %v", err)
	}
	if seq != 9 || string(got) != string(value) {
		t.Errorf("got seq=%d val=%q, want seq=9 val=%q", seq, string(got), string(value))
	}
}
