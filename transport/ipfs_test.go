package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type mockIPFSServer struct {
	t       *testing.T
	server  *httptest.Server
	mu      sync.Mutex
	files   map[string][]byte
	entries map[string][]filesLsEntry
	pins    map[string]bool
	pubsub  []string
	cidLog  []string
}

func newMockIPFSServer(t *testing.T) *mockIPFSServer {
	m := &mockIPFSServer{
		t:       t,
		files:   make(map[string][]byte),
		entries: make(map[string][]filesLsEntry),
		pins:    make(map[string]bool),
	}
	m.entries["/"] = []filesLsEntry{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		p := r.URL.Path
		q := r.URL.Query()
		arg := q.Get("arg")

		// Точные длинные пути должны быть ДО коротких
		switch {
		case strings.HasSuffix(p, "/files/ls"):
			ents, _ := m.entries[arg]
			json.NewEncoder(w).Encode(filesLsResponse{Entries: ents})

		case strings.HasSuffix(p, "/files/read"):
			data, ok := m.files[arg]
			if !ok {
				w.WriteHeader(500)
				return
			}
			w.Write(data)

		case strings.HasSuffix(p, "/files/write"):
			file, _, err := r.FormFile("file")
			if err != nil {
				w.WriteHeader(500)
				return
			}
			data, _ := io.ReadAll(file)
			file.Close()
			m.files[arg] = data
			dir := filepath.Dir(arg)
			name := filepath.Base(arg)
			m.safeAddEntry(dir, name, 0, int64(len(data)))
			for p := dir; p != "/" && p != "."; p = filepath.Dir(p) {
				m.safeAddEntry(filepath.Dir(p), filepath.Base(p), 1, 0)
			}
			json.NewEncoder(w).Encode(map[string]string{})

		case strings.HasSuffix(p, "/files/rm"):
			delete(m.files, arg)
			dir := filepath.Dir(arg)
			name := filepath.Base(arg)
			if ents, ok := m.entries[dir]; ok {
				var kept []filesLsEntry
				for _, e := range ents {
					if e.Name != name {
						kept = append(kept, e)
					}
				}
				m.entries[dir] = kept
			}
			json.NewEncoder(w).Encode(map[string]string{})

		case strings.HasSuffix(p, "/files/mkdir"):
			if _, ok := m.entries[arg]; !ok {
				m.entries[arg] = []filesLsEntry{}
			}
			m.safeAddEntry(filepath.Dir(arg), filepath.Base(arg), 1, 0)
			json.NewEncoder(w).Encode(map[string]string{})

		case strings.HasSuffix(p, "/files/stat"):
			_, fok := m.files[arg]
			_, dok := m.entries[arg]
			if !fok && !dok {
				w.WriteHeader(500)
				return
			}
			json.NewEncoder(w).Encode(filesStatResponse{})

		// Специфичные ДО общих
		case strings.HasSuffix(p, "/pin/add"):
			if arg != "" {
				m.pins[arg] = true
			}
			json.NewEncoder(w).Encode(map[string]string{})

		case strings.HasSuffix(p, "/pin/rm"):
			delete(m.pins, arg)
			json.NewEncoder(w).Encode(map[string]string{})

		case strings.HasSuffix(p, "/pubsub/sub"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"from":"test","data":"ok","topicIDs":["ok"]}`+"\n")

		case strings.HasSuffix(p, "/pubsub/pub"):
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				m.pubsub = append(m.pubsub, string(body))
			}
			json.NewEncoder(w).Encode(map[string]string{})

		// Общие (последними!)
		case strings.HasSuffix(p, "/add"):
			file, _, err := r.FormFile("file")
			if err != nil {
				w.WriteHeader(500)
				return
			}
			data, _ := io.ReadAll(file)
			file.Close()
			cid := fmt.Sprintf("Qm%x", data)
			m.files["/ipfs/"+cid] = data
			m.cidLog = append(m.cidLog, cid)
			json.NewEncoder(w).Encode(addResponse{Name: q.Get("filename"), Hash: cid, Size: fmt.Sprintf("%d", len(data))})

		case strings.HasSuffix(p, "/get"):
			data, ok := m.files["/ipfs/"+arg]
			if !ok {
				w.WriteHeader(500)
				return
			}
			w.Write(data)

		default:
			w.WriteHeader(404)
		}
	}))
	return m
}

func (m *mockIPFSServer) safeAddEntry(dir, name string, typ int, size int64) {
	if m.entries[dir] == nil {
		m.entries[dir] = []filesLsEntry{}
	}
	for _, e := range m.entries[dir] {
		if e.Name == name {
			return
		}
	}
	m.entries[dir] = append(m.entries[dir], filesLsEntry{
		Name: name, Type: typ, Size: size, Hash: "QmMock" + name,
	})
}

func (m *mockIPFSServer) setFile(content string) string {
	data := []byte(content)
	cid := fmt.Sprintf("Qm%x", data)
	m.files["/ipfs/"+cid] = data
	// Также сохраняем под тестовым именем
	m.files["/ipfs/"+cid+"-test"] = data
	m.cidLog = append(m.cidLog, cid)
	return cid
}

func (m *mockIPFSServer) close()       { m.server.Close() }
func (m *mockIPFSServer) addr() string { return m.server.URL }

// Mock Discovery Server
type mockDiscoveryServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	messages map[string]cidMessage
}

func newMockDiscoveryServer() *mockDiscoveryServer {
	m := &mockDiscoveryServer{messages: make(map[string]cidMessage)}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		project := r.URL.Query().Get("project")
		if r.Method == "POST" {
			var msg cidMessage
			json.NewDecoder(r.Body).Decode(&msg)
			m.messages[project] = msg
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/latest") {
			msg, ok := m.messages[project]
			if !ok {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(msg)
			return
		}
		w.WriteHeader(404)
	}))
	return m
}

func (m *mockDiscoveryServer) close()       { m.server.Close() }
func (m *mockDiscoveryServer) addr() string { return m.server.URL }

func createTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0755)
	os.WriteFile(p, []byte(content), 0644)
	return p
}

// ============= MFS =============

func TestIPFSMFS_PushPull(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "mfs_root": "/sync"})
	ld := t.TempDir()
	client.Push(createTestFile(t, ld, "h.txt", "hi"), "h.txt")
	d := filepath.Join(ld, "r.txt")
	client.Pull("h.txt", d)
	data, _ := os.ReadFile(d)
	if string(data) != "hi" {
		t.Errorf("got %q", string(data))
	}
}

func TestIPFSMFS_List(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "mfs_root": "/sync"})
	ld := t.TempDir()
	client.Push(createTestFile(t, ld, "a.txt", "a"), "a.txt")
	client.Push(createTestFile(t, ld, "b.txt", "b"), "b.txt")
	files, _ := client.List("")
	if len(files) != 2 {
		t.Fatalf("expected 2, got %d", len(files))
	}
}

func TestIPFSMFS_Delete(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "mfs_root": "/sync"})
	ld := t.TempDir()
	client.Push(createTestFile(t, ld, "f.txt", "x"), "f.txt")
	client.Delete("f.txt")
	files, _ := client.List("")
	if len(files) != 0 {
		t.Error("expected 0")
	}
}

// ============= PUBSUB =============

func TestIPFSPubSub_Push(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{
		"api": mock.addr(), "pubsub_topic": "/sync/t", "pin": "true",
	})
	ld := t.TempDir()
	err := client.Push(createTestFile(t, ld, "d.txt", "data"), "d.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(mock.pins) == 0 {
		t.Error("expected pin")
	}
	if len(mock.pubsub) == 0 {
		t.Error("expected pubsub message")
	}
}

func TestIPFSPubSub_Pull(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	cid := mock.setFile("remote")
	client, _ := NewIPFSClient(map[string]string{
		"api": mock.addr(), "pubsub_topic": "/sync/t",
	})
	ld := t.TempDir()
	err := client.Pull(cid, filepath.Join(ld, "r.txt"))
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(ld, "r.txt"))
	if string(data) != "remote" {
		t.Errorf("got %q", string(data))
	}
}

func TestIPFSPubSub_FullCycle(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()

	clientA, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "pubsub_topic": "/sync/s", "pin": "true"})
	ldA := t.TempDir()
	clientA.Push(createTestFile(t, ldA, "f.txt", "shared"), "f.txt")

	if len(mock.cidLog) == 0 {
		t.Fatal("no CID")
	}
	cid := mock.cidLog[len(mock.cidLog)-1]

	clientB, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "pubsub_topic": "/sync/s"})
	ldB := t.TempDir()
	err := clientB.Pull(cid, filepath.Join(ldB, "r.txt"))
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(ldB, "r.txt"))
	if string(data) != "shared" {
		t.Errorf("got %q", string(data))
	}
}

// ============= HYBRID =============

func TestIPFSHybrid_PushPull(t *testing.T) {
	mIPFS := newMockIPFSServer(t)
	defer mIPFS.close()
	mDisc := newMockDiscoveryServer()
	defer mDisc.close()

	clientA, _ := NewIPFSClient(map[string]string{
		"api": mIPFS.addr(), "discover_url": mDisc.addr(), "project": "p", "pin": "true",
	})
	ldA := t.TempDir()
	clientA.Push(createTestFile(t, ldA, "d.bin", "hybrid-data"), "d.bin")

	clientB, _ := NewIPFSClient(map[string]string{
		"api": mIPFS.addr(), "discover_url": mDisc.addr(), "project": "p",
	})
	ldB := t.TempDir()
	err := clientB.Pull("ignored", filepath.Join(ldB, "r.txt"))
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(ldB, "r.txt"))
	if string(data) != "hybrid-data" {
		t.Errorf("got %q", string(data))
	}
}

// ============= COMMON =============

func TestIPFS_Name(t *testing.T) {
	c, _ := NewIPFSClient(map[string]string{})
	if c.Name() != "ipfs" {
		t.Errorf("got %q", c.Name())
	}
}

func TestIPFS_Modes(t *testing.T) {
	c1, _ := NewIPFSClient(map[string]string{})
	if c1.mode != modeMFS {
		t.Error("expected MFS")
	}
	c2, _ := NewIPFSClient(map[string]string{"pubsub_topic": "/sync/t"})
	if c2.mode != modePubSub {
		t.Error("expected PubSub")
	}
	c3, _ := NewIPFSClient(map[string]string{"discover_url": "http://h"})
	if c3.mode != modeHybrid {
		t.Error("expected Hybrid")
	}
}

func TestIPFS_Test(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr()})
	if err := client.Test(); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestIPFS_Error(t *testing.T) {
	client, _ := NewIPFSClient(map[string]string{"api": "http://127.0.0.1:19999"})
	if _, err := client.List(""); err == nil {
		t.Error("expected error")
	}
}

func TestIPFS_DataIntegrity(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "mfs_root": "/sync"})
	ld := t.TempDir()
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	src := filepath.Join(ld, "b.bin")
	os.WriteFile(src, data, 0644)
	client.Push(src, "b.bin")
	dst := filepath.Join(ld, "o.bin")
	client.Pull("b.bin", dst)
	res, _ := os.ReadFile(dst)
	if !bytes.Equal(data, res) {
		t.Error("corrupted")
	}
}

func TestIPFS_Overwrite(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "mfs_root": "/sync"})
	ld := t.TempDir()
	createTestFile(t, ld, "v.txt", "v1")
	client.Push(filepath.Join(ld, "v.txt"), "v.txt")
	createTestFile(t, ld, "v.txt", "v2")
	client.Push(filepath.Join(ld, "v.txt"), "v.txt")
	dst := filepath.Join(ld, "r.txt")
	client.Pull("v.txt", dst)
	data, _ := os.ReadFile(dst)
	if string(data) != "v2" {
		t.Errorf("got %q", string(data))
	}
}

func TestIPFS_Subscribe(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "pubsub_topic": "/sync/t"})
	ch, err := client.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if ch == nil {
		t.Error("expected channel")
	}
}

func TestIPFS_SubscribeNoTopic(t *testing.T) {
	client, _ := NewIPFSClient(map[string]string{})
	_, err := client.Subscribe()
	if err == nil {
		t.Error("expected error")
	}
}

func TestIPFS_LastCID(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	client, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "pubsub_topic": "/sync/t"})
	if c := client.LastCID(); c != "" {
		t.Error("expected empty")
	}
	ld := t.TempDir()
	client.Push(createTestFile(t, ld, "f.txt", "d"), "f.txt")
	if c := client.LastCID(); c == "" {
		t.Error("expected non-empty")
	}
}
