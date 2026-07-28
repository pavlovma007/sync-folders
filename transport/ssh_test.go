package transport

import (
	"testing"
	"time"
)

func TestSSHName(t *testing.T) {
	c, _ := NewSSH(map[string]string{"host": "example.com"})
	if c.Name() != "ssh" {
		t.Errorf("expected 'ssh', got %q", c.Name())
	}
}

func TestSSHRemotePathJoin(t *testing.T) {
	tests := []struct {
		remotePath string
		p          string
		expected   string
	}{
		{"/backup", "file.txt", "/backup/file.txt"},
		{"/backup/", "file.txt", "/backup/file.txt"},
		{"", "file.txt", "file.txt"},
		{"/backup", "sub/file.txt", "/backup/sub/file.txt"},
	}

	for _, tt := range tests {
		c := &SSHClient{remotePath: tt.remotePath}
		got := c.remotePathJoin(tt.p)
		if got != tt.expected {
			t.Errorf("remotePathJoin(%q, %q) = %q, want %q", tt.remotePath, tt.p, got, tt.expected)
		}
	}
}

func TestParseLSLine_RegularFile(t *testing.T) {
	line := "-rw-r--r-- 1 root root 1234 Oct 26 10:30 photo.jpg"
	fi := parseLSLine(line, "")
	if fi == nil {
		t.Fatal("parseLSLine returned nil")
	}
	if fi.Name != "photo.jpg" {
		t.Errorf("expected 'photo.jpg', got %q", fi.Name)
	}
	if fi.Path != "photo.jpg" {
		t.Errorf("expected 'photo.jpg', got %q", fi.Path)
	}
	if fi.Size != 1234 {
		t.Errorf("expected size 1234, got %d", fi.Size)
	}
	if fi.IsDir {
		t.Error("expected IsDir=false")
	}
}

func TestParseLSLine_Directory(t *testing.T) {
	line := "drwxr-xr-x 2 root root 4096 Oct 26 10:30 docs"
	fi := parseLSLine(line, "")
	if fi == nil {
		t.Fatal("parseLSLine returned nil")
	}
	if fi.Name != "docs" {
		t.Errorf("expected 'docs', got %q", fi.Name)
	}
	if !fi.IsDir {
		t.Error("expected IsDir=true")
	}
}

func TestParseLSLine_DotAndDotDot(t *testing.T) {
	// . и .. должны быть пропущены
	fi := parseLSLine("drwxr-xr-x 2 root root 4096 Oct 26 10:30 .", "")
	if fi != nil {
		t.Error("expected nil for '.'")
	}
	fi = parseLSLine("drwxr-xr-x 2 root root 4096 Oct 26 10:30 ..", "")
	if fi != nil {
		t.Error("expected nil for '..'")
	}
}

func TestParseLSLine_ShortLine(t *testing.T) {
	// Строка с менее чем 9 полями
	fi := parseLSLine("short line", "")
	if fi != nil {
		t.Error("expected nil for short line")
	}
}

func TestParseLSLine_WithPrefix(t *testing.T) {
	line := "-rw-r--r-- 1 root root 567 Oct 26 10:30 file.txt"
	fi := parseLSLine(line, "subdir")
	if fi == nil {
		t.Fatal("parseLSLine returned nil")
	}
	if fi.Path != "subdir/file.txt" {
		t.Errorf("expected 'subdir/file.txt', got %q", fi.Path)
	}
}

func TestParseLSLine_YearFormat(t *testing.T) {
	// Формат с годом вместо времени
	line := "-rw-r--r-- 1 root root 999 Oct 26  2025 archive.tar.gz"
	fi := parseLSLine(line, "")
	if fi == nil {
		t.Fatal("parseLSLine returned nil")
	}
	if fi.Name != "archive.tar.gz" {
		t.Errorf("expected 'archive.tar.gz', got %q", fi.Name)
	}
	if fi.Size != 999 {
		t.Errorf("expected size 999, got %d", fi.Size)
	}
}

func TestParseLS_FullOutput(t *testing.T) {
	output := `total 16
-rw-r--r-- 1 root root  123 Oct 26 10:30 file1.txt
-rw-r--r-- 1 root root  456 Oct 26 11:00 file2.txt
drwxr-xr-x 2 root root 4096 Oct 26 12:00 subdir`

	files := parseLS(output, "")
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	// Проверяем имена
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
	}
	if !names["file1.txt"] {
		t.Error("expected file1.txt in results")
	}
	if !names["file2.txt"] {
		t.Error("expected file2.txt in results")
	}
	if !names["subdir"] {
		t.Error("expected subdir in results")
	}
	// Проверяем что subdir - директория
	for _, f := range files {
		if f.Name == "subdir" && !f.IsDir {
			t.Error("subdir should be IsDir=true")
		}
	}
}

func TestParseLS_Empty(t *testing.T) {
	files := parseLS("total 0\n", "")
	if len(files) != 0 {
		t.Errorf("expected 0 files for empty listing, got %d", len(files))
	}
}

func TestParseLS_WithTotal(t *testing.T) {
	// Строка "total N" должна быть проигнорирована
	output := "total 8\n-rw-r--r-- 1 root root 100 Oct 26 10:00 data.txt"
	files := parseLS(output, "")
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "data.txt" {
		t.Errorf("expected 'data.txt', got %q", files[0].Name)
	}
}

func TestParseLS_Symlink(t *testing.T) {
	// Символическая ссылка (l в начале)
	line := "lrwxrwxrwx 1 root root 10 Oct 26 10:30 link -> target.txt"
	fi := parseLSLine(line, "")
	if fi == nil {
		t.Fatal("parseLSLine returned nil for symlink")
	}
	if fi.Name != "link" {
		t.Errorf("expected 'link', got %q", fi.Name)
	}
	if fi.Size != 10 {
		t.Errorf("expected size 10, got %d", fi.Size)
	}
}

func TestParseLS_ModTime(t *testing.T) {
	// Вывод ls не содержит года, поэтому парсер использует time.Now() как fallback.
	// Проверяем что mod_time не zero (будет ~текущее время).
	line := "-rw-r--r-- 1 root root 100 Oct 26 10:30 test.bin"
	fi := parseLSLine(line, "")
	if fi == nil {
		t.Fatal("parseLSLine returned nil")
	}
	if fi.ModTime.IsZero() {
		t.Error("expected non-zero ModTime")
	}
	// fallback: year == 0 → time.Now(), поэтому проверяем что год ~текущий
	now := time.Now()
	if fi.ModTime.Year() != now.Year() {
		t.Errorf("expected current year %d, got %d", now.Year(), fi.ModTime.Year())
	}
}

func TestParseLS_WithSpecialChars(t *testing.T) {
	// Имена файлов с пробелами (обёрнутые в кавычки в ls)
	// Этот тест проверяет что парсинг не падает
	line := "-rw-r--r-- 1 root root 200 Oct 26 10:30 my file.txt"
	fi := parseLSLine(line, "")
	if fi == nil {
		// Парсинг может не поддерживать пробелы в именах — это известное ограничение
		// Просто проверяем что не паникует
		return
	}
	// Если распарсилось — проверяем что Name не пустой
	if fi.Name == "" {
		t.Error("expected non-empty name")
	}
}
