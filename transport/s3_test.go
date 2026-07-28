package transport

import (
	"testing"
)

func TestS3Name(t *testing.T) {
	c, _ := NewS3(map[string]string{
		"endpoint": "storage.yandexcloud.net",
		"bucket":   "my-backup",
	})
	if c.Name() != "s3" {
		t.Errorf("expected 's3', got %q", c.Name())
	}
}

func TestS3Key(t *testing.T) {
	tests := []struct {
		prefix     string
		remotePath string
		expected   string
	}{
		{"", "file.txt", "file.txt"},
		{"data/", "file.txt", "data/file.txt"},
		{"data", "file.txt", "data/file.txt"},
		{"backup", "sub/file.txt", "backup/sub/file.txt"},
		{"data/", "", "data"},
		{"", "", ""},
		{"prefix/", "/leading/slash.txt", "prefix/leading/slash.txt"},
	}

	for _, tt := range tests {
		c := &S3Client{prefix: tt.prefix}
		got := c.key(tt.remotePath)
		if got != tt.expected {
			t.Errorf("key(%q) with prefix=%q = %q, want %q",
				tt.remotePath, tt.prefix, got, tt.expected)
		}
	}
}

func TestS3NewRequiredFields(t *testing.T) {
	cfg := map[string]string{
		"endpoint":   "storage.yandexcloud.net",
		"access_key": "AKIAIOSFODNN7EXAMPLE",
		"secret_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"bucket":     "my-bucket",
		"prefix":     "data/",
	}
	c, err := NewS3(cfg)
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if c.endpoint != "storage.yandexcloud.net" {
		t.Errorf("expected endpoint, got %q", c.endpoint)
	}
	if c.accessKey != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("expected accessKey, got %q", c.accessKey)
	}
	if c.secretKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("expected secretKey, got %q", c.secretKey)
	}
	if c.bucket != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", c.bucket)
	}
	if c.prefix != "data/" {
		t.Errorf("expected prefix 'data/', got %q", c.prefix)
	}
}

func TestS3NewDefaults(t *testing.T) {
	c, err := NewS3(map[string]string{
		"endpoint": "minio.example.com",
		"bucket":   "test",
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if c.prefix != "" {
		t.Errorf("expected empty prefix, got %q", c.prefix)
	}
}

func TestS3NameConstant(t *testing.T) {
	c1, _ := NewS3(map[string]string{"endpoint": "a.com", "bucket": "b1"})
	c2, _ := NewS3(map[string]string{"endpoint": "b.com", "bucket": "b2"})
	if c1.Name() != c2.Name() {
		t.Error("Name() should be constant for all S3 instances")
	}
}
