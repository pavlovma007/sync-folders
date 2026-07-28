package filter

import (
	"fmt"
	"sync-folders/transport"

	"github.com/dop251/goja"
)

// Engine выполняет JS-скрипт фильтрации.
type Engine struct {
	name   string
	script string
	vm     *goja.Runtime
}

// New создаёт Engine из JS-кода.
func New(name, script string) *Engine {
	vm := goja.New()

	// Регистрируем объект korni с API
	korniObj := vm.NewObject()
	vm.Set("korni", korniObj)

	return &Engine{
		name:   name,
		script: script,
		vm:     vm,
	}
}

// Run выполняет скрипт фильтрации для списка файлов.
func (e *Engine) Run(files []transport.FileInfo, folder, direction string) ([]transport.FileInfo, error) {
	// Устанавливаем контекст
	korni := e.vm.Get("korni").ToObject(e.vm)
	korni.Set("folder", folder)
	korni.Set("direction", direction)

	// Регистрируем korni.ls
	korni.Set("ls", func(call goja.FunctionCall) goja.Value {
		// TODO: реальный ls
		return e.vm.ToValue([]interface{}{})
	})

	// Преобразуем []transport.FileInfo в []interface{} для JS
	var jsFiles []interface{}
	for _, f := range files {
		jsFiles = append(jsFiles, map[string]interface{}{
			"name":     f.Name,
			"path":     f.Path,
			"size":     f.Size,
			"mod_time": f.ModTime.Unix(),
			"is_dir":   f.IsDir,
		})
	}

	// Выполняем скрипт
	_, err := e.vm.RunString(e.script)
	if err != nil {
		return nil, fmt.Errorf("js parse: %w", err)
	}

	// Вызываем filter(files, context)
	fn, ok := goja.AssertFunction(e.vm.Get("filter"))
	if !ok {
		return nil, fmt.Errorf("filter function not found in script")
	}

	context := e.vm.NewObject()
	context.Set("folder", folder)
	context.Set("direction", direction)

	result, err := fn(goja.Undefined(), e.vm.ToValue(jsFiles), context)
	if err != nil {
		return nil, fmt.Errorf("filter() error: %w", err)
	}

	// Преобразуем результат обратно
	resultFiles, err := e.toFileInfo(result)
	if err != nil {
		return nil, fmt.Errorf("filter result: %w", err)
	}
	return resultFiles, nil
}

func (e *Engine) toFileInfo(val goja.Value) ([]transport.FileInfo, error) {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, nil
	}
	obj := val.Export()
	arr, ok := obj.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", obj)
	}
	var result []transport.FileInfo
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		fi := transport.FileInfo{
			Name:  toString(m["name"]),
			Path:  toString(m["path"]),
			Size:  toInt64(m["size"]),
			IsDir: toBool(m["is_dir"]),
		}
		result = append(result, fi)
	}
	return result, nil
}

func toString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	}
	return 0
}

func toBool(v interface{}) bool {
	b, _ := v.(bool)
	return b
}
