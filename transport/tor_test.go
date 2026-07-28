package transport

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockSocks5Server struct {
	proxy string
	ln    net.Listener
}

func newMockSocks5Server(t *testing.T) *mockSocks5Server {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("socks5 listen: %v", err)
	}
	s := &mockSocks5Server{proxy: ln.Addr().String(), ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleSocks5Conn(conn)
		}
	}()
	return s
}

func (s *mockSocks5Server) close() { s.ln.Close() }

func handleSocks5Conn(client net.Conn) {
	defer client.Close()

	// 1. Приветствие
	buf := make([]byte, 3)
	io.ReadFull(client, buf)
	client.Write([]byte{0x05, 0x00})

	// 2. Запрос на подключение
	req := make([]byte, 4)
	io.ReadFull(client, req)

	// Определяем тип адреса
	var targetAddr string
	switch req[3] {
	case 0x01: // IPv4
		addr := make([]byte, 4)
		io.ReadFull(client, addr)
		port := make([]byte, 2)
		io.ReadFull(client, port)
		targetAddr = fmt.Sprintf("%d.%d.%d.%d:%d", addr[0], addr[1], addr[2], addr[3], (uint16(port[0])<<8)|uint16(port[1]))
	case 0x03: // Domain
		lenByte := make([]byte, 1)
		io.ReadFull(client, lenByte)
		domain := make([]byte, lenByte[0])
		io.ReadFull(client, domain)
		port := make([]byte, 2)
		io.ReadFull(client, port)
		targetAddr = fmt.Sprintf("%s:%d", string(domain), (uint16(port[0])<<8)|uint16(port[1]))
	}

	// Подключаемся к цели
	target, err := net.Dial("tcp", targetAddr)
	if err != nil {
		return
	}
	defer target.Close()

	// Успешный ответ
	client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x7f, 0x00, 0x00, 0x01, 0x00, 0x00})

	// Прокси данных в обе стороны
	errc := make(chan error, 2)
	go func() { _, err := io.Copy(target, client); errc <- err }()
	go func() { _, err := io.Copy(client, target); errc <- err }()
	<-errc
}

func TestTorName(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	inner, _ := NewIPFSClient(map[string]string{"api": mock.addr()})
	tp, _ := NewTorProxy("127.0.0.1:9050", inner)
	name := tp.Name()
	if !strings.HasPrefix(name, "tor+") {
		t.Errorf("expected prefix 'tor+', got %q", name)
	}
}

func TestTorCheckRunning(t *testing.T) {
	s := newMockSocks5Server(t)
	defer s.close()
	if err := TorCheck(s.proxy); err != nil {
		t.Fatalf("TorCheck: %v", err)
	}
}

func TestTorCheckNotRunning(t *testing.T) {
	if err := TorCheck("127.0.0.1:1"); err == nil {
		t.Error("expected error")
	}
}

func TestTorCheckEmpty(t *testing.T) {
	if err := TorCheck(""); err == nil {
		t.Error("expected error")
	}
}

func TestTorProxyHTTP(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello tor"))
	}))
	defer target.Close()

	s := newMockSocks5Server(t)
	defer s.close()

	client, _ := NewSOCKS5HTTPClient(s.proxy)
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "hello tor" {
		t.Errorf("got %q", string(data))
	}
}

func TestTorHTTPTransport(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]testFileEntry{{Name: "f.txt", Size: 1, ModTime: 1700000000}})
	}))
	defer target.Close()

	s := newMockSocks5Server(t)
	defer s.close()

	inner, _ := NewHTTPClient(map[string]string{"url": target.URL})
	wrapped, _ := WrapWithProxy(inner, s.proxy)
	files, err := wrapped.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1, got %d", len(files))
	}
}

func TestTorWithIPFS(t *testing.T) {
	mock := newMockIPFSServer(t)
	defer mock.close()
	s := newMockSocks5Server(t)
	defer s.close()

	inner, _ := NewIPFSClient(map[string]string{"api": mock.addr(), "mfs_root": "/sync"})
	wrapped, _ := WrapWithProxy(inner, s.proxy)

	ld := t.TempDir()
	src := filepath.Join(ld, "t.txt")
	os.WriteFile(src, []byte("data"), 0644)

	if err := wrapped.Push(src, "t.txt"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	files, _ := wrapped.List("")
	if len(files) != 1 {
		t.Errorf("expected 1, got %d", len(files))
	}
	wrapped.Delete("t.txt")
	files, _ = wrapped.List("")
	if len(files) != 0 {
		t.Error("expected 0 after delete")
	}
}

func TestTorCreateTransport(t *testing.T) {
	_, err := CreateTorTransport(map[string]string{"proxy": "", "inner_type": "ipfs"})
	if err == nil {
		t.Error("expected error")
	}
	_, err = CreateTorTransport(map[string]string{"proxy": "127.0.0.1:9050", "inner_type": ""})
	if err == nil {
		t.Error("expected error")
	}
}

func TestTorGetThroughProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxy data"))
	}))
	defer target.Close()
	s := newMockSocks5Server(t)
	defer s.close()
	data, err := httpGetThroughProxy(s.proxy, target.URL)
	if err != nil {
		t.Fatalf("httpGet: %v", err)
	}
	if string(data) != "proxy data" {
		t.Errorf("got %q", string(data))
	}
}

func TestTorSaveFile(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("saved"))
	}))
	defer target.Close()
	s := newMockSocks5Server(t)
	defer s.close()
	ld := t.TempDir()
	dest := filepath.Join(ld, "f.txt")
	if err := saveFileThroughProxy(s.proxy, target.URL, dest); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "saved" {
		t.Errorf("got %q", string(data))
	}
}
