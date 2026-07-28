package transport

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TorProxy — прокси-слой через Tor SOCKS5 для любого транспорта.
//
// Оборачивает существующий Transport и перенаправляет все сетевые вызовы
// через SOCKS5-прокси (Tor). Для HTTP-транспортов используется
// SOCKS5-совместимый http.Client.
//
// Конфиг:
//
//	транспорт:
//	  type: tor
//	  config:
//	    proxy: "socks5://127.0.0.1:9050"
//	    inner:
//	      type: ssh
//	      config:
//	        host: "xyz.onion"
//	        user: "admin"
//	        key: "~/.ssh/id_ed25519"
type TorProxy struct {
	proxyAddr string
	inner     Transport
	client    *http.Client
}

// NewSOCKS5HTTPClient создаёт http.Client с SOCKS5-прокси.
// Если proxyAddr пустой — возвращает обычный клиент.
func NewSOCKS5HTTPClient(proxyAddr string) (*http.Client, error) {
	if proxyAddr == "" {
		return &http.Client{Timeout: 30 * time.Second}, nil
	}

	// Парсим адрес: "socks5://127.0.0.1:9050" → "127.0.0.1:9050"
	addr := proxyAddr
	if strings.HasPrefix(addr, "socks5://") {
		addr = strings.TrimPrefix(addr, "socks5://")
	}

	// Создаём SOCKS5 dialer через raw TCP (без external dependencies)
	dialer := &socks5Dialer{addr: addr}

	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}, nil
}

// ————— Raw SOCKS5 клиент (без внешних зависимостей) —————

type socks5Dialer struct {
	addr string
}

func (d *socks5Dialer) Dial(network, addr string) (net.Conn, error) {
	// Подключаемся к SOCKS5-прокси
	proxyConn, err := net.DialTimeout("tcp", d.addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("socks5 dial proxy %s: %w", d.addr, err)
	}

	// SOCKS5 handshake
	// 1. Приветствие: NO AUTH (0x00)
	if _, err := proxyConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("socks5 greet: %w", err)
	}

	// 2. Читаем ответ сервера
	resp := make([]byte, 2)
	if _, err := io.ReadFull(proxyConn, resp); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("socks5 greet resp: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		proxyConn.Close()
		return nil, fmt.Errorf("socks5: server rejected auth method: %v", resp)
	}

	// 3. Запрос на подключение
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("socks5 split: %w", err)
	}

	// IP or domain
	var atyp byte
	var dstAddr []byte
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			atyp = 0x01 // IPv4
			dstAddr = ip4
		} else {
			atyp = 0x04 // IPv6
			dstAddr = ip.To16()
		}
	} else {
		atyp = 0x03 // Domain name
		dstAddr = []byte{byte(len(host))}
		dstAddr = append(dstAddr, []byte(host)...)
	}

	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	req := []byte{0x05, 0x01, 0x00, atyp}
	req = append(req, dstAddr...)
	req = append(req, byte(port>>8), byte(port))

	if _, err := proxyConn.Write(req); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("socks5 connect req: %w", err)
	}

	// 4. Читаем ответ
	connectResp := make([]byte, 4)
	if _, err := io.ReadFull(proxyConn, connectResp); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("socks5 connect resp: %w", err)
	}
	if connectResp[1] != 0x00 {
		proxyConn.Close()
		return nil, fmt.Errorf("socks5: connection refused: code %d", connectResp[1])
	}

	// Читаем оставшуюся часть ответа (BND.ADDR + BND.PORT)
	var restLen int
	switch connectResp[3] {
	case 0x01:
		restLen = 4 + 2 // IPv4 + port
	case 0x03:
		restLen = int(connectResp[3]) + 2
	case 0x04:
		restLen = 16 + 2 // IPv6 + port
	}
	if restLen > 0 {
		rest := make([]byte, restLen)
		io.ReadFull(proxyConn, rest)
	}

	return proxyConn, nil
}

// ————— TorProxy —————

// NewTorProxy создаёт прокси-обёртку над транспортом.
func NewTorProxy(proxyAddr string, inner Transport) (*TorProxy, error) {
	httpClient, err := NewSOCKS5HTTPClient(proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("tor proxy: %w", err)
	}

	return &TorProxy{
		proxyAddr: proxyAddr,
		inner:     inner,
		client:    httpClient,
	}, nil
}

func (tp *TorProxy) Name() string {
	return "tor+" + tp.inner.Name()
}

// List возвращает список файлов через прокси.
func (tp *TorProxy) List(remotePath string) ([]FileInfo, error) {
	return tp.inner.List(remotePath)
}

// Push загружает файл через прокси.
func (tp *TorProxy) Push(localPath, remotePath string) error {
	return tp.inner.Push(localPath, remotePath)
}

// Pull скачивает файл через прокси.
func (tp *TorProxy) Pull(remotePath, localPath string) error {
	return tp.inner.Pull(remotePath, localPath)
}

// Delete удаляет файл через прокси.
func (tp *TorProxy) Delete(remotePath string) error {
	return tp.inner.Delete(remotePath)
}

// Test проверяет прокси и транспорт.
func (tp *TorProxy) Test() error {
	if err := TorCheck(tp.proxyAddr); err != nil {
		return fmt.Errorf("tor: %w", err)
	}
	return tp.inner.Test()
}

// TorCheck проверяет что SOCKS5-прокси (Tor) доступен.
func TorCheck(proxyAddr string) error {
	if proxyAddr == "" {
		return fmt.Errorf("proxy address is empty")
	}

	addr := proxyAddr
	if strings.HasPrefix(addr, "socks5://") {
		addr = strings.TrimPrefix(addr, "socks5://")
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("tor not available: %w", err)
	}
	defer conn.Close()

	// Проверяем что это SOCKS5-сервер (шлём приветствие)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return fmt.Errorf("tor check write: %w", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("tor check read: %w", err)
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("not a SOCKS5 proxy")
	}

	return nil
}

// ————— Factory helper —————

// CreateTorTransport создаёт транспорт с Tor-прокси из конфига.
// Ожидается структура:
//
//	{
//	  "proxy": "socks5://127.0.0.1:9050",
//	  "inner_type": "ssh",
//	  "inner_config": { ... }
//	}
func CreateTorTransport(cfg map[string]string) (Transport, error) {
	proxyAddr := cfg["proxy"]
	if proxyAddr == "" {
		return nil, fmt.Errorf("tor: proxy address required")
	}

	innerType := cfg["inner_type"]
	if innerType == "" {
		return nil, fmt.Errorf("tor: inner transport type required")
	}

	// Собираем конфиг для внутреннего транспорта
	innerCfg := make(map[string]string)
	for k, v := range cfg {
		if strings.HasPrefix(k, "inner_") {
			innerCfg[strings.TrimPrefix(k, "inner_")] = v
		}
	}

	// Если конфиг пустой — пробуем найти inner_config как JSON
	// или используем остаток cfg без tor-специфичных ключей
	if len(innerCfg) == 0 {
		for k, v := range cfg {
			if k != "proxy" && k != "inner_type" {
				innerCfg[k] = v
			}
		}
	}

	inner, err := Factory(innerType, innerCfg)
	if err != nil {
		return nil, fmt.Errorf("tor: inner transport %s: %w", innerType, err)
	}

	return NewTorProxy(proxyAddr, inner)
}

// SetProxyHTTPClient заменяет http.Client у транспорта, если он поддерживает
// интерфейс HTTPClientSetter. Используется для внедрения SOCKS5-клиента.
type HTTPClientSetter interface {
	SetHTTPClient(client *http.Client)
}

// SetHTTPClient implements HTTPClientSetter for relevant transports.
// Добавляется к HTTPClient, MySQLClient, WebDAVClient, IPFSClient.

// HTTPClientGetter возвращает http.Client, если транспорт его поддерживает.
type HTTPClientGetter interface {
	GetHTTPClient() *http.Client
}

// ————— Для HTTPClient (http.go) —————

func (h *HTTPClient) SetHTTPClient(client *http.Client) {
	h.client = client
}

func (h *HTTPClient) GetHTTPClient() *http.Client {
	return h.client
}

// ————— Для MySQLClient (mysql.go) —————

func (m *MySQLClient) SetHTTPClient(client *http.Client) {
	m.client = client
}

func (m *MySQLClient) GetHTTPClient() *http.Client {
	return m.client
}

// ————— Для WebDAVClient (webdav.go) —————

func (w *WebDAVClient) SetHTTPClient(client *http.Client) {
	w.client = client
}

func (w *WebDAVClient) GetHTTPClient() *http.Client {
	return w.client
}

// ————— Для IPFSClient (ipfs.go) —————

func (ic *IPFSClient) SetHTTPClient(client *http.Client) {
	ic.client = client
}

func (ic *IPFSClient) GetHTTPClient() *http.Client {
	return ic.client
}

// WrapWithProxy заменяет http.Client у транспорта, если он поддерживает
// HTTPClientSetter. Возвращает тот же транспорт с SOCKS5-клиентом.
func WrapWithProxy(transp Transport, proxyAddr string) (Transport, error) {
	if proxyAddr == "" {
		return transp, nil
	}

	setter, ok := transp.(HTTPClientSetter)
	if !ok {
		// Если транспорт не поддерживает замену клиента,
		// оборачиваем его в TorProxy
		return NewTorProxy(proxyAddr, transp)
	}

	socks5Client, err := NewSOCKS5HTTPClient(proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("wrap proxy: %w", err)
	}

	setter.SetHTTPClient(socks5Client)
	return transp, nil
}

// ————— Setup file downloads via SOCKS5 —————

// httpGetThroughProxy делает HTTP GET запрос через SOCKS5-прокси.
func httpGetThroughProxy(proxyAddr, targetURL string) ([]byte, error) {
	client, err := NewSOCKS5HTTPClient(proxyAddr)
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(targetURL)
	if err != nil {
		return nil, fmt.Errorf("http get proxy: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// httpPostThroughProxy делает HTTP POST запрос через SOCKS5-прокси.
func httpPostThroughProxy(proxyAddr, targetURL, contentType string, body io.Reader) ([]byte, error) {
	client, err := NewSOCKS5HTTPClient(proxyAddr)
	if err != nil {
		return nil, err
	}

	resp, err := client.Post(targetURL, contentType, body)
	if err != nil {
		return nil, fmt.Errorf("http post proxy: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// saveFileThroughProxy скачивает файл через SOCKS5-прокси и сохраняет на диск.
func saveFileThroughProxy(proxyAddr, fileURL, localPath string) error {
	data, err := httpGetThroughProxy(proxyAddr, fileURL)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(localPath, data, 0644)
}
