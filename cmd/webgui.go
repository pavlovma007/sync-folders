package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync-folders/core"
	"time"

	"gopkg.in/yaml.v3"
)

func RunGUI() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/folders", handleFolders)
	mux.HandleFunc("/api/configs", handleConfigs)
	mux.HandleFunc("/api/sync", handleSync)
	mux.HandleFunc("/api/folder/add", handleFolderAdd)
	mux.HandleFunc("/api/folder/remove", handleFolderRemove)
	mux.HandleFunc("/api/folder/clear", handleFolderClear)
	mux.HandleFunc("/api/config/add", handleConfigAdd)
	mux.HandleFunc("/api/config/remove", handleConfigRemove)
	mux.HandleFunc("/api/config/download", handleConfigDownload)
	mux.HandleFunc("/", handleIndex)

	lastPing := time.Now()
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		lastPing = time.Now()
		jsonResponse(w, map[string]string{"status": "ok"})
	})

	// Случайный свободный порт
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot find free port: %v\n", err)
		os.Exit(1)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	server := &http.Server{Addr: addr, Handler: mux}

	// Сервер в фоне
	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "GUI error: %v\n", err)
		}
	}()

	fmt.Printf("\n🌐  GUI: http://%s/\n", addr)
	fmt.Println("    Server auto-shutdown 15s after browser tab closes")
	fmt.Print("  Opening browser... ")

	openBrowser("http://" + addr + "/")
	fmt.Println("done")

	// Ждём первый ping от браузера (до 30 сек)
	for i := 0; i < 300; i++ {
		if !lastPing.IsZero() && time.Since(lastPing) < 5*time.Second {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Следим за ping'ами, если 15с тишины — shutdown
	for {
		time.Sleep(2 * time.Second)
		if time.Since(lastPing) > 15*time.Second {
			fmt.Println("\n  Browser closed — shutting down")
			os.Exit(0)
		}
	}
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "linux":
		p, err := exec.LookPath("xdg-open")
		if err != nil {
			break
		}
		cmd := exec.Command(p, url)
		cmd.Env = append(os.Environ(), "DISPLAY="+os.Getenv("DISPLAY"))
		cmd.Start()
	case "darwin":
		exec.Command("open", url).Start()
	case "windows":
		exec.Command("cmd", "/c", "start", url).Start()
	}
}

func jsonResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func handleFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := core.ListFolders()
	if err != nil {
		jsonResponse(w, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, folders)
}

func handleConfigs(w http.ResponseWriter, r *http.Request) {
	cfgs, err := core.ListConfigs()
	if err != nil {
		jsonResponse(w, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, cfgs)
}

func handleFolderAdd(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	path := r.FormValue("path")
	if err := core.AddFolder(name, path); err != nil {
		jsonResponse(w, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func handleFolderRemove(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if err := core.RemoveFolder(name); err != nil {
		jsonResponse(w, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func handleFolderClear(w http.ResponseWriter, r *http.Request) {
	folders, _ := core.ListFolders()
	for _, f := range folders {
		core.RemoveFolder(f.Name)
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func handleConfigAdd(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, _, err := r.FormFile("yaml")
	if err != nil {
		jsonResponse(w, map[string]string{"error": "upload yaml file"})
		return
	}
	defer file.Close()

	tmpFile, err := os.CreateTemp("", "sync-config-*.yaml")
	if err != nil {
		jsonResponse(w, map[string]string{"error": err.Error()})
		return
	}
	defer os.Remove(tmpFile.Name())

	io.Copy(tmpFile, file)
	tmpFile.Close()

	if err := core.AddConfig(tmpFile.Name()); err != nil {
		jsonResponse(w, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func handleConfigRemove(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if err := core.RemoveConfig(name); err != nil {
		jsonResponse(w, map[string]string{"error": err.Error()})
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

func handleConfigDownload(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "missing name parameter", http.StatusBadRequest)
		return
	}

	cfg, err := core.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sc, ok := cfg.Syncs[name]
	if !ok {
		http.Error(w, fmt.Sprintf("config %q not found", name), http.StatusNotFound)
		return
	}

	data, err := yaml.Marshal(sc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.yaml"`, name))
	w.Write(data)
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		core.Daemon(0)
		_ = ctx
	}()
	jsonResponse(w, map[string]string{"status": "started"})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

const indexHTML = `<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>sync-folders</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
* { margin:0; padding:0; box-sizing:border-box; }
body { font-family: system-ui, sans-serif; background: #0f0f14; color: #e0e0e0; padding: 20px; }
h1 { font-size: 1.5em; margin-bottom: 20px; color: #fff; }
.card { background: #1a1a24; border-radius: 8px; padding: 16px; margin-bottom: 16px; }
.card h2 { font-size: 1em; color: #888; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 1px; }
table { width: 100%; border-collapse: collapse; }
td { padding: 8px 4px; border-bottom: 1px solid #222; }
input[type=text] { width: 100%; padding: 6px 8px; background: #25253a; border: 1px solid #333; border-radius: 4px; color: #fff; }
.btn { display: inline-block; padding: 6px 14px; border-radius: 4px; cursor: pointer; font-size: 0.9em; border: none; }
.btn-primary { background: #4a6cf7; color: #fff; }
.btn-danger { background: #e54c4c; color: #fff; }
.btn-sm { padding: 3px 8px; font-size: 0.8em; }
.btn-warn { background: #e5a54c; color: #fff; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.actions { margin-top: 8px; display: flex; gap: 8px; align-items: center; }
.actions input { flex: 1; }
@media(max-width: 600px) { .grid { grid-template-columns: 1fr; } }
</style></head>
<body>
<h1>📁 sync-folders</h1>
<div class="grid">
<div>
<div class="card">
<h2>Папки <span id="folderCount"></span></h2>
<div id="folderList"></div>
<div class="actions">
<input type="text" id="folderName" placeholder="name">
<input type="text" id="folderPath" placeholder="/path">
<button class="btn btn-primary" onclick="addFolder()">+</button>
</div>
<div id="clearBtn" style="margin-top:8px">
<button class="btn btn-warn btn-sm" onclick="clearFolders()">Очистить все папки</button>
</div>
</div></div>
<div>
<div class="card"><h2>Конфиги</h2><div id="configList"></div>
<div class="actions">
<input type="file" id="configFile" accept=".yaml" style="flex:1">
<button class="btn btn-primary" onclick="addConfig()">+</button>
</div></div>
</div>
</div>
<div class="card"><button class="btn btn-primary" onclick="runSync()">▶ Запустить синхронизацию</button></div>

<script>
async function load() {
  const f = await (await fetch('/api/folders')).json();
  var html = '';
  if(Array.isArray(f)) {
    html = '<table>' + f.map(x => '<tr><td>'+x.name+'</td><td>'+x.path+'</td><td><button class="btn btn-danger btn-sm" onclick="removeFolder(\''+x.name+'\')">x</button></td></tr>').join('') + '</table>';
    document.getElementById('folderCount').textContent = '('+f.length+')';
  }
  document.getElementById('folderList').innerHTML = html || '<i>empty</i>';
  
  const c = await (await fetch('/api/configs')).json();
  var html2 = '';
  if(c && typeof c === 'object') {
    html2 = '<table>' + Object.entries(c).map(([k,v]) => '<tr><td>'+k+'</td><td><small>folder: '+v.Folder+'</small></td><td>'+v.Transport.Type+'</td><td><button class="btn btn-sm btn-primary" onclick="downloadConfig(\''+k+'\')">⬇</button> <button class="btn btn-sm btn-danger" onclick="removeConfig(\''+k+'\')">x</button></td></tr>').join('') + '</table>';
  }
  document.getElementById('configList').innerHTML = html2 || '<i>empty</i>';
}
async function addFolder() {
  var n = document.getElementById('folderName').value, p = document.getElementById('folderPath').value;
  if(!n||!p) return;
  await fetch('/api/folder/add?'+new URLSearchParams({name:n,path:p}));
  document.getElementById('folderName').value=''; document.getElementById('folderPath').value=''; load();
}
async function removeFolder(n) { await fetch('/api/folder/remove?'+new URLSearchParams({name:n})); load(); }
async function clearFolders() { if(confirm('Очистить все папки?')) { await fetch('/api/folder/clear'); load(); } }
async function addConfig() {
  var f = document.getElementById('configFile').files[0]; if(!f) return;
  var fd = new FormData(); fd.append('yaml', f);
  await fetch('/api/config/add', {method:'POST',body:fd}); load();
}
async function downloadConfig(name) {
  var a = document.createElement('a');
  a.href = '/api/config/download?name=' + encodeURIComponent(name);
  a.download = name + '.yaml';
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}
async function removeConfig(name) {
  if(!confirm('Remove config "'+name+'"?')) return;
  await fetch('/api/config/remove?' + new URLSearchParams({name: name}));
  load();
}
async function runSync() { await fetch('/api/sync'); alert('Sync started'); }
setInterval(function() { fetch('/api/ping').catch(function(){}); }, 5000);
load();
</script>
</body></html>`
