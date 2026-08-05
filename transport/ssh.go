package transport

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	host       string
	port       string
	user       string
	keyPath    string
	remotePath string
	client     *ssh.Client
}

func NewSSH(cfg map[string]string) (*SSHClient, error) {
	if cfg["port"] == "" {
		cfg["port"] = "22"
	}
	return &SSHClient{
		host:       cfg["host"],
		port:       cfg["port"],
		user:       cfg["user"],
		keyPath:    cfg["key"],
		remotePath: cfg["remote_path"],
	}, nil
}

func (s *SSHClient) Name() string { return "ssh" }

func (s *SSHClient) connect() error {
	if s.client != nil {
		return nil
	}
	key, err := os.ReadFile(filepath.Clean(s.keyPath))
	if err != nil {
		return fmt.Errorf("ssh key %s: %w", s.keyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("ssh parse key: %w", err)
	}
	addr := s.host + ":" + s.port
	config := &ssh.ClientConfig{
		User:            s.user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	s.client = client
	return nil
}

func (s *SSHClient) remotePathJoin(p string) string {
	if s.remotePath == "" {
		return p
	}
	return strings.TrimRight(s.remotePath, "/") + "/" + strings.TrimLeft(p, "/")
}

func (s *SSHClient) List(remotePath string) ([]FileInfo, error) {
	if err := s.connect(); err != nil {
		return nil, err
	}

	fullPath := s.remotePathJoin(remotePath)
	session, err := s.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	// Используем ls -la для получения подробной информации
	cmd := fmt.Sprintf("ls -la '%s' 2>/dev/null || echo '__EMPTY__'", fullPath)
	out, err := session.Output(cmd)
	if err != nil {
		return nil, fmt.Errorf("ssh ls: %w", err)
	}

	output := string(out)
	if strings.Contains(output, "__EMPTY__") || strings.TrimSpace(output) == "" {
		return []FileInfo{}, nil
	}

	return parseLS(output, remotePath), nil
}

// parseLS парсит вывод ls -la.
func parseLS(output, prefix string) []FileInfo {
	lines := strings.Split(output, "\n")
	var result []FileInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Пропускаем "total N" и родительские директории
		if strings.HasPrefix(line, "total") || strings.Contains(line, "..") {
			continue
		}

		fi := parseLSLine(line, prefix)
		if fi != nil {
			result = append(result, *fi)
		}
	}
	return result
}

// parseLSLine парсит одну строку ls -la.
// Формат: -rw-r--r-- 1 user group size month day HH:MM name
// Формат с годом: -rw-r--r-- 1 user group size month day  YYYY name
func parseLSLine(line, prefix string) *FileInfo {
	parts := strings.Fields(line)
	if len(parts) < 9 {
		return nil
	}

	// Первый символ: d = директория, - = файл, l = симлинк
	mode := parts[0]
	if len(mode) == 0 {
		return nil
	}

	isDir := mode[0] == 'd'
	size, _ := strconv.ParseInt(parts[4], 10, 64)

	// Время всегда в parts[5:8] — это месяц, день, время или год
	// Имя файла — parts[8] (для симлинков parts[9:] содержит "-> target")
	timeStr := strings.Join(parts[5:8], " ")
	name := parts[8]

	if name == "." || name == ".." {
		return nil
	}

	// Парсим время (пример: "Oct 26 10:30" или "Oct 26 2025")
	var modTime time.Time
	for _, f := range []string{"Jan 2 15:04", "Jan 2 2006", "Jan _2 15:04"} {
		if t, err := time.Parse(f, timeStr); err == nil {
			modTime = t
			break
		}
	}
	if modTime.Year() == 0 {
		modTime = time.Now()
	}

	relPath := name
	if prefix != "" {
		relPath = strings.TrimRight(prefix, "/") + "/" + name
	}

	return &FileInfo{
		Name:    name,
		Path:    relPath,
		Size:    size,
		ModTime: modTime,
		IsDir:   isDir,
	}
}

// Push копирует файл на удалённую машину через SCP.
func (s *SSHClient) Push(localPath, remotePath string) error {
	if err := s.connect(); err != nil {
		return err
	}

	// Сначала создаём удалённую директорию
	s.mkdirp(filepath.Dir(remotePath))

	fullRemote := s.remotePathJoin(remotePath)
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("read local %s: %w", localPath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// SCP protocol: scp -t remote_path
	// Протокол:
	//   локальный → C<perms> <size> <name>\n
	//   удалённый  → \x00 (ack)
	//   локальный → данные
	//   локальный → \x00
	//   удалённый  → \x00 (done)
	perms := "0644"
	scpCmd := fmt.Sprintf("scp -t '%s'", fullRemote)

	// НЕ задаём session.Stdin = file — используем StdinPipe для протокола SCP
	w, err := session.StdinPipe()
	if err != nil {
		return err
	}
	defer w.Close()

	r, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	session.Stderr = os.Stderr

	if err := session.Start(scpCmd); err != nil {
		return fmt.Errorf("scp start: %w", err)
	}

	br := bufio.NewReader(r)

	// Отправляем заголовок C<perms> <size> <name>
	header := fmt.Sprintf("C%s %d %s\n", perms, stat.Size(), stat.Name())
	if _, err := w.Write([]byte(header)); err != nil {
		return fmt.Errorf("scp header: %w", err)
	}
	// Ждём ack (\x00)
	if err := scpReadAck(br); err != nil {
		return fmt.Errorf("scp ack header: %w", err)
	}

	// Отправляем содержимое файла
	data, _ := io.ReadAll(file)
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("scp data: %w", err)
	}
	// Завершающий \x00 (EOF marker)
	if _, err := w.Write([]byte{0}); err != nil {
		return fmt.Errorf("scp terminator: %w", err)
	}
	// Ждём ack после данных
	if err := scpReadAck(br); err != nil {
		return fmt.Errorf("scp data ack: %w", err)
	}
	// Конец передачи: E\n
	if _, err := w.Write([]byte("E\n")); err != nil {
		return fmt.Errorf("scp end: %w", err)
	}
	// Ждём финальный ack
	if err := scpReadAck(br); err != nil {
		return fmt.Errorf("scp end ack: %w", err)
	}

	return session.Wait()
}

// scpReadAck читает SCP-подтверждение (байт \x00 или \x01 с сообщением об ошибке).
func scpReadAck(r *bufio.Reader) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	switch b {
	case 0:
		return nil
	case 1:
		msg, _ := r.ReadString('\n')
		return fmt.Errorf("scp remote error: %s", msg)
	default:
		return fmt.Errorf("scp unexpected response: %d", b)
	}
}

// Pull копирует файл с удалённой машины через SCP.
func (s *SSHClient) Pull(remotePath, localPath string) error {
	if err := s.connect(); err != nil {
		return err
	}

	fullRemote := s.remotePathJoin(remotePath)

	// Используем scp -f (source mode) через ssh
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	scpCmd := fmt.Sprintf("scp -f '%s'", fullRemote)
	out, err := session.Output(scpCmd)
	if err != nil {
		return fmt.Errorf("scp pull: %w, out: %s", err, string(out))
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(localPath, out, 0644)
}

// Delete удаляет файл на удалённой машине.
func (s *SSHClient) Delete(remotePath string) error {
	if err := s.connect(); err != nil {
		return err
	}

	fullRemote := s.remotePathJoin(remotePath)
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	return session.Run(fmt.Sprintf("rm -f '%s'", fullRemote))
}

// Test проверяет соединение.
func (s *SSHClient) Test() error {
	if err := s.connect(); err != nil {
		return err
	}
	session, err := s.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run("echo ok")
}

// mkdirp создаёт удалённую директорию рекурсивно.
func (s *SSHClient) mkdirp(dir string) {
	if dir == "" || dir == "." || dir == "/" {
		return
	}
	session, err := s.client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()
	session.Run(fmt.Sprintf("mkdir -p '%s'", s.remotePathJoin(dir)))
}
