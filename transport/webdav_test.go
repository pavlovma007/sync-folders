package transport

import (
	"testing"
)

func TestWebDAVName(t *testing.T) {
	c, _ := NewWebDAV(map[string]string{"url": "https://example.com/dav/"})
	if c.Name() != "webdav" {
		t.Errorf("expected 'webdav', got %q", c.Name())
	}
}

func TestWebDAVURL(t *testing.T) {
	tests := []struct {
		baseURL  string
		rootPath string
		p        string
		expected string
	}{
		{
			baseURL:  "https://example.com/dav/",
			rootPath: "",
			p:        "file.txt",
			expected: "https://example.com/dav/file.txt",
		},
		{
			baseURL:  "https://example.com/dav",
			rootPath: "",
			p:        "file.txt",
			expected: "https://example.com/dav/file.txt",
		},
		{
			baseURL:  "https://example.com/",
			rootPath: "sync",
			p:        "sub/file.txt",
			expected: "https://example.com/sync/sub/file.txt",
		},
		{
			baseURL:  "https://example.com",
			rootPath: "/sync/",
			p:        "file.txt",
			expected: "https://example.com/sync/file.txt",
		},
		{
			baseURL:  "https://example.com/remote.php/dav/files/user/",
			rootPath: "",
			p:        "",
			expected: "https://example.com/remote.php/dav/files/user/",
		},
		{
			baseURL:  "https://example.com/",
			rootPath: "",
			p:        "",
			expected: "https://example.com/",
		},
	}

	for _, tt := range tests {
		c := &WebDAVClient{baseURL: tt.baseURL, rootPath: tt.rootPath}
		got := c.url(tt.p)
		if got != tt.expected {
			t.Errorf("url(%q) with baseURL=%q rootPath=%q = %q, want %q",
				tt.p, tt.baseURL, tt.rootPath, got, tt.expected)
		}
	}
}

func TestWebDAVNewRequiredFields(t *testing.T) {
	c, err := NewWebDAV(map[string]string{
		"url":         "https://nextcloud.example.com/dav/",
		"user":        "admin",
		"password":    "secret",
		"remote_path": "sync",
	})
	if err != nil {
		t.Fatalf("NewWebDAV: %v", err)
	}
	if c.baseURL != "https://nextcloud.example.com/dav/" {
		t.Errorf("expected baseURL, got %q", c.baseURL)
	}
	if c.user != "admin" {
		t.Errorf("expected user 'admin', got %q", c.user)
	}
	if c.password != "secret" {
		t.Errorf("expected password 'secret', got %q", c.password)
	}
	if c.rootPath != "sync" {
		t.Errorf("expected rootPath 'sync', got %q", c.rootPath)
	}
}

func TestWebDAVNameConstant(t *testing.T) {
	c1, _ := NewWebDAV(map[string]string{"url": "https://a.com/"})
	c2, _ := NewWebDAV(map[string]string{"url": "https://b.com/"})
	if c1.Name() != c2.Name() {
		t.Error("Name() should be constant for all WebDAV instances")
	}
}
