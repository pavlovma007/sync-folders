package transport

import (
	"testing"
)

func TestFTPName(t *testing.T) {
	c, _ := NewFTP(map[string]string{"host": "example.com", "port": "21"})
	if c.Name() != "ftp" {
		t.Errorf("expected 'ftp', got %q", c.Name())
	}
}

func TestFTPNewDefaultPort(t *testing.T) {
	// Проверяем что порт по умолчанию не устанавливается в NewFTP (это responsibility вызывающей стороны)
	c, _ := NewFTP(map[string]string{"host": "example.com"})
	if c.port != "" {
		t.Errorf("expected empty port, got %q", c.port)
	}
}

func TestFTPNewWithAllFields(t *testing.T) {
	cfg := map[string]string{
		"host":        "ftp.example.com",
		"port":        "21",
		"user":        "testuser",
		"password":    "secret",
		"remote_path": "/backup",
	}
	c, err := NewFTP(cfg)
	if err != nil {
		t.Fatalf("NewFTP: %v", err)
	}
	if c.host != "ftp.example.com" {
		t.Errorf("expected host 'ftp.example.com', got %q", c.host)
	}
	if c.port != "21" {
		t.Errorf("expected port '21', got %q", c.port)
	}
	if c.user != "testuser" {
		t.Errorf("expected user 'testuser', got %q", c.user)
	}
	if c.password != "secret" {
		t.Errorf("expected password 'secret', got %q", c.password)
	}
	if c.rootPath != "/backup" {
		t.Errorf("expected rootPath '/backup', got %q", c.rootPath)
	}
}

func TestFTPNameReturn(t *testing.T) {
	// Проверяем Name() у разных экземпляров
	c1, _ := NewFTP(map[string]string{"host": "a.com"})
	c2, _ := NewFTP(map[string]string{"host": "b.com"})
	if c1.Name() != c2.Name() {
		t.Error("Name() should be constant for all FTP instances")
	}
}
