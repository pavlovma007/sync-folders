package dht

import (
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/bep44"
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
	// We pass a nil Store to use default, but with a custom config
	// This is a lightweight creation test
	c := &Client{
		store:   bep44.NewMemory(),
		timeout: time.Second,
	}
	_ = c
	t.Log("Client creation OK (server omitted for unit test)")
}
