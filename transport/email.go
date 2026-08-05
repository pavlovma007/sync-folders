package transport

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EmailClient — почтовый транспорт.
// Использует IMAP для чтения/списка/удаления, SMTP для отправки.
// Файлы сжимаются gzip, метаданные — в Subject письма (JSON).
//
// Конфиг:
//
//	imap:  "imap.example.com:993"   — IMAP-сервер (TLS)
//	smtp:  "smtp.example.com:587"   — SMTP-сервер (STARTTLS)
//	user:  "user@example.com"
//	pass:  "${EMAIL_PASS}"
//	folder: "INBOX"                  — папка (по умолч. INBOX)
//	compress: "true"                 — gzip-сжатие (по умолч. true)
type EmailClient struct {
	imapHost        string
	smtpHost        string
	user            string
	pass            string
	folder          string
	compress        bool
	selfSignedCerts bool
	maxSize         int64 // максимальный размер файла для Push
}

// emailMeta — метаданные в Subject письма.
type emailMeta struct {
	Path string `json:"path"`
	LMT  string `json:"lmt"` // last modification time, формат "2006-01-02 15:04:05"
}

const (
	defaultMaxSize = 25 * 1024 * 1024 // 25MB
)

func NewEmailClient(cfg map[string]string) (*EmailClient, error) {
	if cfg["imap"] == "" || cfg["smtp"] == "" || cfg["user"] == "" || cfg["pass"] == "" {
		return nil, fmt.Errorf("email: imap, smtp, user and pass required")
	}

	compress := true
	if cfg["compress"] == "false" {
		compress = false
	}

	selfSignedCerts := false
	if cfg["self_signed_certs"] == "true" {
		selfSignedCerts = true
	}

	folder := cfg["folder"]
	if folder == "" {
		folder = "INBOX"
	}

	maxSize := int64(defaultMaxSize)
	if cfg["max_size"] != "" {
		if v, err := strconv.ParseInt(cfg["max_size"], 10, 64); err == nil && v > 0 {
			maxSize = v
		}
	}

	return &EmailClient{
		imapHost:        cfg["imap"],
		smtpHost:        cfg["smtp"],
		user:            cfg["user"],
		pass:            cfg["pass"],
		folder:          folder,
		compress:        compress,
		selfSignedCerts: selfSignedCerts,
		maxSize:         maxSize,
	}, nil
}

func (e *EmailClient) Name() string { return "email" }

// ─── IMAP helper (raw protocol) ─────────────────────────────

type imapConn struct {
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
	tag  int
}

func (e *EmailClient) dialIMAP(host string) (*imapConn, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: e.selfSignedCerts}
	conn, err := tls.Dial("tcp", host, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("imap dial: %w", err)
	}
	c := &imapConn{
		conn: conn,
		br:   bufio.NewReader(conn),
		bw:   bufio.NewWriter(conn),
	}
	// Читаем greeting
	_, err = c.readline()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("imap greeting: %w", err)
	}
	return c, nil
}

func (c *imapConn) nextTag() string {
	c.tag++
	return fmt.Sprintf("a%04d", c.tag)
}

func (c *imapConn) writeln(format string, args ...interface{}) error {
	line := fmt.Sprintf(format, args...)
	if _, err := c.bw.WriteString(line + "\r\n"); err != nil {
		return err
	}
	return c.bw.Flush()
}

func (c *imapConn) readline() (string, error) {
	line, err := c.br.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readLiteral читает literal {size} с последующими size байтами.
func (c *imapConn) readLiteral(size int) (string, error) {
	data := make([]byte, size)
	_, err := io.ReadFull(c.br, data)
	if err != nil {
		return "", err
	}
	// После literal обычно идёт закрывающая скобка и \r\n
	return string(data), nil
}

// command отправляет команду и читает ответы до tagged-ответа.
// Возвращает все untagged строки ответа.
func (c *imapConn) command(format string, args ...interface{}) ([]string, error) {
	tag := c.nextTag()
	cmd := fmt.Sprintf(format, args...)
	if err := c.writeln("%s %s", tag, cmd); err != nil {
		return nil, err
	}

	var resp []string
	for {
		line, err := c.readline()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, tag) {
			if strings.Contains(line, "OK") {
				return resp, nil
			}
			return resp, fmt.Errorf("imap %s: %s", cmd, line)
		}
		resp = append(resp, line)
	}
}

// login выполняет LOGIN.
func (c *imapConn) login(user, pass string) error {
	_, err := c.command("LOGIN %s %s", user, pass)
	return err
}

// selectMailbox выполняет SELECT.
func (c *imapConn) selectMailbox(folder string) error {
	_, err := c.command(`SELECT "%s"`, folder)
	return err
}

// search отправляет SEARCH и возвращает ID сообщений.
func (c *imapConn) search(criteria string) ([]int, error) {
	resp, err := c.command("SEARCH %s", criteria)
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, line := range resp {
		if strings.HasPrefix(line, "* SEARCH") {
			parts := strings.Fields(line)
			for _, p := range parts[2:] {
				if id, err := strconv.Atoi(p); err == nil {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids, nil
}

// fetchHeader получает Subject из заголовка письма.
func (c *imapConn) fetchHeader(id int) (string, error) {
	resp, err := c.command("FETCH %d BODY.PEEK[HEADER.FIELDS (Subject)]", id)
	if err != nil {
		return "", err
	}
	return extractSubjectFromIMAPResponse(resp), nil
}

// fetchBody получает полное тело письма.
func (c *imapConn) fetchBody(id int) ([]byte, error) {
	resp, err := c.command("FETCH %d BODY[]", id)
	if err != nil {
		return nil, err
	}
	return extractBodyFromIMAPResponse(resp)
}

// delete отмечает сообщение на удаление и expunge.
func (c *imapConn) delete(id int) error {
	_, err := c.command("STORE %d +FLAGS (\\Deleted)", id)
	if err != nil {
		return err
	}
	_, err = c.command("EXPUNGE")
	return err
}

func (c *imapConn) logout() {
	c.writeln("%s LOGOUT", c.nextTag())
}

func (c *imapConn) close() {
	c.logout()
	c.conn.Close()
}

// extractSubjectFromIMAPResponse ищет Subject в ответе FETCH HEADER.
func extractSubjectFromIMAPResponse(resp []string) string {
	for _, line := range resp {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Subject:") {
			s := strings.TrimPrefix(line, "Subject:")
			return strings.TrimSpace(s)
		}
		// Subject может быть на следующей строке после "Subject:"
		if strings.HasPrefix(line, "Subject:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// extractBodyFromIMAPResponse извлекает тело письма из ответа FETCH BODY[].
func extractBodyFromIMAPResponse(resp []string) ([]byte, error) {
	var buf bytes.Buffer
	inLiteral := false
	literalSize := 0

	for _, line := range resp {
		if !inLiteral {
			// Ищем literal: "{size}"
			if idx := strings.Index(line, "{"); idx >= 0 {
				endIdx := strings.Index(line[idx:], "}")
				if endIdx >= 0 {
					sizeStr := line[idx+1 : idx+endIdx]
					if sz, err := strconv.Atoi(sizeStr); err == nil {
						literalSize = sz
						inLiteral = true
						// Текст до literal уже в line, но это часть IMAP ответа, не данных
						continue
					}
				}
			}
		} else {
			// Это literal data. IMAP разбивает literal на "строки" по \r\n.
			// Восстанавливаем разделитель между кусками, иначе MIME-структура
			// ломается (заголовки письма сливаются в одну строку).
			// Обрезаем строго до literalSize байт.
			remaining := literalSize - buf.Len()
			if remaining <= 0 {
				inLiteral = false
				continue
			}
			// Добавляем \r\n перед куском, если это не первый кусок literal.
			if buf.Len() > 0 && !strings.HasPrefix(line, "\r\n") && remaining > 2 {
				buf.WriteString("\r\n")
				remaining--
			}
			chunk := line
			if len(chunk) > remaining {
				chunk = chunk[:remaining]
			}
			buf.WriteString(chunk)
			if buf.Len() >= literalSize {
				inLiteral = false
			}
		}
	}

	if buf.Len() == 0 {
		// Пробуем другой подход: склеиваем все строки
		for _, line := range resp {
			buf.WriteString(line)
			buf.WriteString("\r\n")
		}
	}

	return buf.Bytes(), nil
}

// ─── SMTP отправка ─────────────────────────────────────────

// sendMail отправляет письмо с вложением через SMTP.
func (e *EmailClient) sendMail(subject string, data []byte, filename string) error {
	var body bytes.Buffer

	// Multipart mixed конверт
	writer := multipart.NewWriter(&body)
	boundary := writer.Boundary()

	// Заголовки
	header := textproto.MIMEHeader{}
	header.Set("From", e.user)
	header.Set("To", e.user)
	header.Set("Subject", subject)
	header.Set("MIME-Version", "1.0")
	header.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%q", boundary))

	// Создаём часть с вложением
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"application/gzip"},
		"Content-Disposition":       {fmt.Sprintf("attachment; filename=%q", filename)},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return fmt.Errorf("email create part: %w", err)
	}

	// Записываем данные (уже gzip) в base64 — заголовок выше обещает base64.
	// Без явного кодирования postfix перекодирует 8bit данные при доставке,
	// и вложение придёт в другом виде.
	if _, err := part.Write([]byte(base64.StdEncoding.EncodeToString(data))); err != nil {
		return fmt.Errorf("email write data: %w", err)
	}
	writer.Close()

	// Отправляем через SMTP
	// Собираем полное письмо
	var msg bytes.Buffer
	for k, v := range header {
		for _, val := range v {
			fmt.Fprintf(&msg, "%s: %s\r\n", k, val)
		}
	}
	msg.WriteString("\r\n")
	msg.Write(body.Bytes())

	// SMTP с кастомным TLS (поддержка self_signed_certs).
	// smtp.SendMail не позволяет задать tls.Config, поэтому делаем вручную.
	serverName := hostWithoutPort(e.smtpHost)

	conn, err := net.Dial("tcp", e.smtpHost)
	if err != nil {
		return fmt.Errorf("email smtp dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, serverName)
	if err != nil {
		return fmt.Errorf("email smtp client: %w", err)
	}

	// STARTTLS с учётом self_signed_certs (если сервер его поддерживает)
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: e.selfSignedCerts,
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("email smtp starttls: %w", err)
		}
	}

	auth := smtp.PlainAuth("", e.user, e.pass, serverName)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("email smtp auth: %w", err)
	}

	if err := client.Mail(e.user); err != nil {
		return fmt.Errorf("email smtp mail: %w", err)
	}
	if err := client.Rcpt(e.user); err != nil {
		return fmt.Errorf("email smtp rcpt: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email smtp data: %w", err)
	}
	if _, err := w.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("email smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email smtp close: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("email smtp quit: %w", err)
	}
	return nil
}

func hostWithoutPort(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// ─── MIME helpers ───────────────────────────────────────────

// parseEmailMeta парсит Subject как JSON метаданные.
func parseEmailMeta(subject string) (*emailMeta, error) {
	var m emailMeta
	if err := json.Unmarshal([]byte(subject), &m); err != nil {
		return nil, err
	}
	if m.Path == "" {
		return nil, fmt.Errorf("empty path in metadata")
	}
	return &m, nil
}

// formatModTime форматирует время в формат для Subject.
func formatModTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// parseModTime парсит время из формата Subject.
func parseModTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", s)
}

// gzipData сжимает данные.
func gzipData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gunzipData разжимает данные.
func gunzipData(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	defer r.Close()
	return io.ReadAll(r)
}

// ─── Transport interface ────────────────────────────────────

// ErrFileTooLarge возвращается когда файл превышает лимит размера.
var ErrFileTooLarge = fmt.Errorf("file too large for email transport")

// List получает список файлов из почтового ящика.
// Парсит Subject каждого письма как JSON {path, lmt}.
func (e *EmailClient) List(remotePath string) ([]FileInfo, error) {
	conn, err := e.dialIMAP(e.imapHost)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	if err := conn.login(e.user, e.pass); err != nil {
		return nil, fmt.Errorf("email list login: %w", err)
	}
	if err := conn.selectMailbox(e.folder); err != nil {
		return nil, fmt.Errorf("email list select: %w", err)
	}

	ids, err := conn.search("ALL")
	if err != nil {
		return nil, fmt.Errorf("email list search: %w", err)
	}

	var result []FileInfo
	for _, id := range ids {
		subject, err := conn.fetchHeader(id)
		if err != nil {
			continue
		}
		meta, err := parseEmailMeta(subject)
		if err != nil {
			continue // письма с не-JSON Subject игнорируем
		}
		lmt, err := parseModTime(meta.LMT)
		if err != nil {
			lmt = time.Now()
		}
		result = append(result, FileInfo{
			Name:    filepath.Base(meta.Path),
			Path:    meta.Path,
			Size:    0, // размер не храним в Subject
			ModTime: lmt,
			IsDir:   false,
		})
	}
	return result, nil
}

// Push отправляет файл как письмо через SMTP.
// Файл сжимается gzip, метаданные (path, lmt) — в Subject.
func (e *EmailClient) Push(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("email push read: %w", err)
	}

	if int64(len(data)) > e.maxSize {
		return ErrFileTooLarge
	}

	var attachData []byte
	attachName := filepath.Base(remotePath)
	meta := emailMeta{
		Path: remotePath,
		LMT:  formatModTime(time.Now()),
	}

	if e.compress {
		compressed, err := gzipData(data)
		if err != nil {
			return fmt.Errorf("email push gzip: %w", err)
		}
		attachData = compressed
		attachName += ".gz"
	} else {
		attachData = data
	}

	subjectBytes, _ := json.Marshal(meta)
	subject := string(subjectBytes)

	// TODO: Обрезка Subject если >998 символов (ограничение RFC).
	// Нужно протестировать на реальном хостинге — есть ли проблема.
	// if len(subject) > 998 {
	// 	shortPath := fmt.Sprintf("%x", []byte(remotePath))
	// 	if len(shortPath) > 32 {
	// 		shortPath = shortPath[:32]
	// 	}
	// 	meta.Path = shortPath
	// 	shortSubject, _ := json.Marshal(meta)
	// 	subject = string(shortSubject)
	// }

	if err := e.sendMail(subject, attachData, attachName); err != nil {
		return fmt.Errorf("email push: %w", err)
	}

	// Rate limiting
	time.Sleep(500 * time.Millisecond)

	return nil
}

// Pull скачивает файл из почты по remotePath.
// Ищет письмо с Subject, содержащим path, извлекает и распаковывает вложение.
func (e *EmailClient) Pull(remotePath, localPath string) error {
	conn, err := e.dialIMAP(e.imapHost)
	if err != nil {
		return err
	}
	defer conn.close()

	if err := conn.login(e.user, e.pass); err != nil {
		return fmt.Errorf("email pull login: %w", err)
	}
	if err := conn.selectMailbox(e.folder); err != nil {
		return fmt.Errorf("email pull select: %w", err)
	}

	ids, err := conn.search("ALL")
	if err != nil {
		return fmt.Errorf("email pull search: %w", err)
	}

	for _, id := range ids {
		subject, err := conn.fetchHeader(id)
		if err != nil {
			continue
		}
		meta, err := parseEmailMeta(subject)
		if err != nil {
			continue
		}
		if meta.Path != remotePath {
			continue
		}

		// Нашли нужное письмо — скачиваем тело
		body, err := conn.fetchBody(id)
		if err != nil {
			return fmt.Errorf("email pull fetch %d: %w", id, err)
		}

		// Извлекаем вложение из MIME
		attachments, err := extractAttachments(body)
		if err != nil {
			return fmt.Errorf("email pull extract: %w", err)
		}
		if len(attachments) == 0 {
			return fmt.Errorf("email pull: no attachment found")
		}

		data := attachments[0]

		// Если сжато — распаковываем
		if e.compress {
			uncompressed, err := gunzipData(data)
			if err != nil {
				return fmt.Errorf("email pull gunzip: %w", err)
			}
			data = uncompressed
		}

		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(localPath, data, 0644)
	}

	return fmt.Errorf("email pull: file %q not found in mailbox", remotePath)
}

// Delete удаляет письмо (помечает \Deleted и EXPUNGE).
func (e *EmailClient) Delete(remotePath string) error {
	conn, err := e.dialIMAP(e.imapHost)
	if err != nil {
		return err
	}
	defer conn.close()

	if err := conn.login(e.user, e.pass); err != nil {
		return fmt.Errorf("email delete login: %w", err)
	}
	if err := conn.selectMailbox(e.folder); err != nil {
		return fmt.Errorf("email delete select: %w", err)
	}

	ids, err := conn.search("ALL")
	if err != nil {
		return fmt.Errorf("email delete search: %w", err)
	}

	for _, id := range ids {
		subject, err := conn.fetchHeader(id)
		if err != nil {
			continue
		}
		meta, err := parseEmailMeta(subject)
		if err != nil {
			continue
		}
		if meta.Path == remotePath {
			return conn.delete(id)
		}
	}

	return fmt.Errorf("email delete: file %q not found", remotePath)
}

// Test проверяет подключение к IMAP.
func (e *EmailClient) Test() error {
	conn, err := e.dialIMAP(e.imapHost)
	if err != nil {
		return err
	}
	defer conn.close()
	return conn.login(e.user, e.pass)
}

// ─── MIME attachment extraction ─────────────────────────────

// extractAttachments извлекает вложения из сырого MIME-сообщения.
// Упрощённая реализация: ищет Content-Type и Content-Disposition,
// затем извлекает данные между boundary.
func extractAttachments(raw []byte) ([][]byte, error) {
	text := string(raw)

	// Ищем boundary из Content-Type
	boundary := ""
	lines := strings.Split(text, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "content-type:") {
			if idx := strings.Index(line, "boundary="); idx >= 0 {
				boundary = line[idx+9:]
				boundary = strings.Trim(boundary, `"'"`)
				break
			}
			// boundary может быть на следующей строке
		}
	}

	if boundary == "" {
		// Нет multipart — всё тело это один attachment
		// Ищем пустую строку (конец заголовков)
		for i, line := range lines {
			if line == "" {
				data := []byte(strings.Join(lines[i+1:], "\r\n"))
				return [][]byte{data}, nil
			}
		}
		return nil, fmt.Errorf("no attachment found")
	}

	// Разделяем по boundary
	parts := strings.Split(text, "--"+boundary)
	var attachments [][]byte

	for _, part := range parts {
		if part == "" {
			continue
		}
		partsLines := strings.Split(part, "\r\n")
		// Ищем Content-Disposition: attachment и Content-Transfer-Encoding
		isAttachment := false
		isBase64 := false
		headerEnd := -1
		for i, line := range partsLines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "content-disposition:") && strings.Contains(lower, "attachment") {
				isAttachment = true
			}
			if strings.Contains(lower, "content-transfer-encoding:") && strings.Contains(lower, "base64") {
				isBase64 = true
			}
			if line == "" && i > 0 {
				headerEnd = i
				break
			}
		}
		if isAttachment && headerEnd > 0 {
			data := []byte(strings.Join(partsLines[headerEnd+1:], "\r\n"))
			// Убираем trailing -- (конец multipart)
			data = bytes.TrimRight(data, "\r\n-")
			// Если вложение в base64 — декодируем
			if isBase64 {
				decoded, err := base64.StdEncoding.DecodeString(string(data))
				if err != nil {
					return nil, fmt.Errorf("decode attachment base64: %w", err)
				}
				data = decoded
			}
			attachments = append(attachments, data)
		}
	}

	if len(attachments) == 0 {
		return nil, fmt.Errorf("no attachment found in multipart message")
	}
	return attachments, nil
}
