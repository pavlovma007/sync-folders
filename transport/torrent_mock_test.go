package transport

import (
	"testing"
)

func TestMockTorrentClient(t *testing.T) {
	mock := NewMockTorrentClient()
	hash, err := mock.AddMagnet("magnet:?xt=urn:btih:test", "/tmp/test")
	if err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
	info, err := mock.GetInfo(hash)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if info.State != "seeding" {
		t.Errorf("state: got %q, want seeding", info.State)
	}
	if info.Hash != hash {
		t.Errorf("hash: got %q, want %q", info.Hash, hash)
	}

	// Delete
	if err := mock.Delete(hash, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := mock.GetInfo(hash); err == nil {
		t.Error("expected error after delete")
	}

	// List after delete should be empty
	list, err := mock.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

func TestMockDHTClient(t *testing.T) {
	mock := NewMockDHTClient()
	pub := []byte("test-pub-key-32-bytes-long!")
	priv := []byte("test-priv-key-64-bytes-long!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")

	err := mock.Put(pub, priv, "salt", 1, []byte("value1"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	val, seq, err := mock.Get(pub, "salt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "value1" || seq != 1 {
		t.Errorf("got %q/%d, want value1/1", string(val), seq)
	}

	// Get not found
	_, _, err = mock.Get([]byte("nonexistent"), "salt")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}

	// Overwrite with higher seq
	mock.Put(pub, priv, "salt", 2, []byte("value2"))
	val, seq, _ = mock.Get(pub, "salt")
	if seq != 2 {
		t.Errorf("seq: got %d, want 2", seq)
	}
}

func TestMockTorrentClientAddTorrentFile(t *testing.T) {
	mock := NewMockTorrentClient()
	hash, err := mock.AddTorrentFile([]byte("torrent data"), "/tmp")
	if err != nil {
		t.Fatalf("AddTorrentFile: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
}
