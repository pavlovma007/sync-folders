package filter

import (
	"sync-folders/transport"
	"testing"
)

func TestFilterJS(t *testing.T) {
	script := `
	function filter(files, ctx) {
		return files.filter(function(f) {
			return f.size < 100;
		});
	}`
	e := New("send", script)

	files := []transport.FileInfo{
		{Name: "small.txt", Size: 50},
		{Name: "large.txt", Size: 200},
		{Name: "tiny.txt", Size: 10},
	}

	result, err := e.Run(files, "/tmp", "send")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 files after filter, got %d", len(result))
	}

	// Проверяем что отфильтровались только маленькие
	for _, f := range result {
		if f.Size >= 100 {
			t.Errorf("file %s with size %d should have been filtered out", f.Name, f.Size)
		}
	}
}

func TestFilterEmptyJS(t *testing.T) {
	// Пустой скрипт — все файлы проходят
	script := `function filter(files, ctx) { return files; }`
	e := New("send", script)

	files := []transport.FileInfo{
		{Name: "a.txt", Size: 1000},
	}

	result, err := e.Run(files, "/tmp", "send")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 file, got %d", len(result))
	}
}

func TestFilterByName(t *testing.T) {
	script := `
	function filter(files, ctx) {
		return files.filter(function(f) {
			return f.name.endsWith('.jpg') || f.name.endsWith('.png');
		});
	}`
	e := New("receive", script)

	files := []transport.FileInfo{
		{Name: "photo.jpg", Size: 500},
		{Name: "doc.pdf", Size: 300},
		{Name: "image.png", Size: 200},
		{Name: "script.exe", Size: 1000},
	}

	result, err := e.Run(files, "/tmp", "receive")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 files (.jpg, .png), got %d", len(result))
	}
}

func TestFilterSyntaxError(t *testing.T) {
	// Невалидный JS — должна быть ошибка
	script := `function filter(files, ctx) { return files; `
	e := New("send", script)

	_, err := e.Run([]transport.FileInfo{}, "/tmp", "send")
	if err == nil {
		t.Error("expected syntax error, got nil")
	}
}
