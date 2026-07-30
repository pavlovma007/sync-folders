package dht

import (
	"testing"
)

func TestSignVerify(t *testing.T) {
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := &Manifest{
		Seq:       42,
		Magnet:    "magnet:?xt=urn:btih:abc123",
		Timestamp: 1700000000,
		FilesHash: "def456",
	}

	_, sig, err := m.Sign(priv)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if !m.Verify(pub, sig) {
		t.Error("Verify failed for valid signature")
	}

	// Tampered manifest should fail
	m2 := &Manifest{Seq: 43, Magnet: "magnet:?xt=urn:btih:abc123", Timestamp: 1700000000, FilesHash: "def456"}
	if m2.Verify(pub, sig) {
		t.Error("Verify should fail for tampered manifest")
	}
}

func TestGenerateKeySize(t *testing.T) {
	pub, priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(pub) != 32 {
		t.Errorf("public key: got %d, want 32", len(pub))
	}
	if len(priv) != 64 {
		t.Errorf("private key: got %d, want 64", len(priv))
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	m := &Manifest{
		Seq:       1,
		Magnet:    "magnet:?xt=urn:btih:test",
		Timestamp: 1000,
		FilesHash: "abc",
	}
	data, err := m.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := UnmarshalManifest(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Seq != m2.Seq || m.Magnet != m2.Magnet || m.Timestamp != m2.Timestamp || m.FilesHash != m2.FilesHash {
		t.Errorf("roundtrip: got %+v, want %+v", m2, m)
	}
}
