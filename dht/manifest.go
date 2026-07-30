package dht

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
)

// Manifest публикуется в DHT как BEP-44 mutable item (JSON, ≤500 байт).
type Manifest struct {
	Seq       int64  `json:"seq"`
	Magnet    string `json:"magnet"`
	Timestamp int64  `json:"ts"`
	FilesHash string `json:"files_hash"`
}

// Marshal сериализует манифест в JSON.
func (m *Manifest) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// Sign подписывает манифест Ed25519-ключом.
// Возвращает публичный ключ и подпись.
func (m *Manifest) Sign(priv ed25519.PrivateKey) (ed25519.PublicKey, []byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, nil, fmt.Errorf("manifest sign marshal: %w", err)
	}
	sig := ed25519.Sign(priv, data)
	return priv.Public().(ed25519.PublicKey), sig, nil
}

// Verify проверяет подпись манифеста.
func (m *Manifest) Verify(pub ed25519.PublicKey, sig []byte) bool {
	data, err := json.Marshal(m)
	if err != nil {
		return false
	}
	return ed25519.Verify(pub, data, sig)
}

// GenerateKey генерирует новую Ed25519 пару для DHT.
func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// UnmarshalManifest десериализует JSON в Manifest.
func UnmarshalManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	return &m, nil
}

// SaltForProject возвращает salt для BEP-44 ключа.
func SaltForProject(project string) string {
	return "sync-folders:" + project
}
