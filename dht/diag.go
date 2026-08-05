package dht

import "fmt"

// Diag — временный диагностический метод для отладки интеграционного теста
// (см. docker/scenarios/25-dht-network). Возвращает состояние DHT-сервера.
func (c *Client) Diag() string {
	if c.server == nil {
		return "server=nil"
	}
	return fmt.Sprintf("nodes=%d addr=%v", c.server.NumNodes(), c.server.Addr())
}
