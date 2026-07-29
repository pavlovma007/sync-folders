package transport

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestCert creates a self-signed TLS certificate for testing.
func newTestCert() tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost", "127.0.0.1"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}

// ─── Mock IMAP сервер ───────────────────────────────────────

type mockIMAPMsg struct {
	Subject string
	Body    []byte
}

type mockIMAPServer struct {
	ln     net.Listener
	msgs   []mockIMAPMsg
	mu     sync.Mutex
	tlsCfg *tls.Config
}

func newMockIMAPServer(t *testing.T, msgs []mockIMAPMsg) *mockIMAPServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock IMAP listen: %v", err)
	}
	s := &mockIMAPServer{
		ln:     ln,
		msgs:   msgs,
		tlsCfg: &tls.Config{Certificates: []tls.Certificate{newTestCert()}},
	}
	go s.serve()
	return s
}

func (s *mockIMAPServer) addr() string {
	return s.ln.Addr().String()
}

func (s *mockIMAPServer) close() {
	s.ln.Close()
}

func (s *mockIMAPServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		tlsConn := tls.Server(conn, s.tlsCfg)
		go s.handleConn(tlsConn)
	}
}

func (s *mockIMAPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	bw := bufio.NewWriter(conn)

	fmt.Fprintf(bw, "* OK Mock IMAP ready\r\n")
	bw.Flush()

	authenticated := false
	selected := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		tag := parts[0]
		cmd := strings.ToUpper(parts[1])

		switch cmd {
		case "LOGIN":
			authenticated = true
			fmt.Fprintf(bw, "%s OK LOGIN completed\r\n", tag)
		case "SELECT":
			selected = true
			fmt.Fprintf(bw, "* 1 EXISTS\r\n* 0 RECENT\r\n%s OK SELECT completed\r\n", tag)
		case "SEARCH":
			s.mu.Lock()
			var ids []string
			for i := range s.msgs {
				ids = append(ids, fmt.Sprintf("%d", i+1))
			}
			s.mu.Unlock()
			fmt.Fprintf(bw, "* SEARCH %s\r\n", strings.Join(ids, " "))
			fmt.Fprintf(bw, "%s OK SEARCH completed\r\n", tag)
		case "FETCH":
			if len(parts) < 3 {
				fmt.Fprintf(bw, "%s BAD invalid fetch\r\n", tag)
				bw.Flush()
				continue
			}
			msgID := 0
			fmt.Sscanf(parts[2], "%d", &msgID)
			msgID--

			s.mu.Lock()
			if msgID < 0 || msgID >= len(s.msgs) {
				s.mu.Unlock()
				fmt.Fprintf(bw, "%s BAD invalid message ID\r\n", tag)
				bw.Flush()
				continue
			}
			msg := s.msgs[msgID]
			s.mu.Unlock()

			if strings.Contains(line, "HEADER.FIELDS") {
				header := fmt.Sprintf("Subject: %s\r\n\r\n", msg.Subject)
				fmt.Fprintf(bw, "* %d FETCH (BODY[HEADER.FIELDS (\"Subject\")] {%d}\r\n%s)\r\n", msgID+1, len(header), header)
			} else {
				fmt.Fprintf(bw, "* %d FETCH (BODY[] {%d}\r\n%s)\r\n", msgID+1, len(msg.Body), string(msg.Body))
			}
			fmt.Fprintf(bw, "%s OK FETCH completed\r\n", tag)
		case "STORE":
			fmt.Fprintf(bw, "%s OK STORE completed\r\n", tag)
		case "EXPUNGE":
			fmt.Fprintf(bw, "%s OK EXPUNGE completed\r\n", tag)
		case "LOGOUT":
			fmt.Fprintf(bw, "* BYE Logging out\r\n%s OK LOGOUT completed\r\n", tag)
			return
		default:
			_ = authenticated
			_ = selected
			fmt.Fprintf(bw, "%s OK %s completed\r\n", tag, cmd)
		}
		bw.Flush()
	}
}

// ─── Mock SMTP сервер ───────────────────────────────────────

type mockSMTPServer struct {
	ln       net.Listener
	received [][]byte
	mu       sync.Mutex
}

func newMockSMTPServer(t *testing.T) *mockSMTPServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock SMTP listen: %v", err)
	}
	s := &mockSMTPServer{ln: ln}
	go s.serve()
	return s
}

func (s *mockSMTPServer) addr() string {
	return s.ln.Addr().String()
}

func (s *mockSMTPServer) close() {
	s.ln.Close()
}

func (s *mockSMTPServer) lastMessage() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.received) == 0 {
		return nil
	}
	return s.received[len(s.received)-1]
}

func (s *mockSMTPServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleSMTP(conn)
	}
}

func (s *mockSMTPServer) handleSMTP(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	bw := bufio.NewWriter(conn)

	fmt.Fprintf(bw, "220 Mock SMTP ready\r\n")
	bw.Flush()

	var msg bytes.Buffer
	inData := false

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				fmt.Fprintf(bw, "250 OK: message accepted\r\n")
				bw.Flush()
				s.mu.Lock()
				cp := make([]byte, msg.Len())
				copy(cp, msg.Bytes())
				s.received = append(s.received, cp)
				s.mu.Unlock()
				msg.Reset()
				inData = false
			} else {
				msg.WriteString(line)
				msg.WriteString("\r\n")
			}
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
			fmt.Fprintf(bw, "250-Hello\r\n")
			fmt.Fprintf(bw, "250-AUTH LOGIN PLAIN\r\n")
			fmt.Fprintf(bw, "250 OK\r\n")
		case strings.HasPrefix(upper, "AUTH"):
			parts := strings.Fields(line)
			if len(parts) >= 4 && strings.EqualFold(parts[1], "PLAIN") {
				fmt.Fprintf(bw, "235 Authentication successful\r\n")
			} else if len(parts) >= 4 && strings.EqualFold(parts[1], "LOGIN") {
				fmt.Fprintf(bw, "334 VXNlcm5hbWU6\r\n") // base64 "Username:"
				resp, _ := br.ReadString('\n')
				_ = resp
				fmt.Fprintf(bw, "334 UGFzc3dvcmQ6\r\n") // base64 "Password:"
				resp, _ = br.ReadString('\n')
				_ = resp
				fmt.Fprintf(bw, "235 Authentication successful\r\n")
			} else {
				fmt.Fprintf(bw, "235 Authentication successful\r\n")
			}
		case strings.HasPrefix(upper, "MAIL FROM:"):
			fmt.Fprintf(bw, "250 OK\r\n")
		case strings.HasPrefix(upper, "RCPT TO:"):
			fmt.Fprintf(bw, "250 OK\r\n")
		case strings.HasPrefix(upper, "DATA"):
			fmt.Fprintf(bw, "354 Start mail input\r\n")
			inData = true
		case strings.HasPrefix(upper, "QUIT"):
			fmt.Fprintf(bw, "221 Bye\r\n")
			bw.Flush()
			return
		default:
			fmt.Fprintf(bw, "250 OK\r\n")
		}
		bw.Flush()
	}
}

// ─── Helpers ────────────────────────────────────────────────

func buildEmailMessage(subject string, data []byte, compress bool) []byte {
	var buf bytes.Buffer
	boundary := "testboundary123"

	fmt.Fprintf(&buf, "From: test@example.com\r\n")
	fmt.Fprintf(&buf, "To: test@example.com\r\n")
	fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary)
	fmt.Fprintf(&buf, "\r\n")

	attachData := data
	attachName := "file.bin"
	if compress {
		var cbuf bytes.Buffer
		w := gzip.NewWriter(&cbuf)
		w.Write(data)
		w.Close()
		attachData = cbuf.Bytes()
		attachName = "file.bin.gz"
	}

	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Type: application/gzip\r\n")
	fmt.Fprintf(&buf, "Content-Disposition: attachment; filename=\"%s\"\r\n", attachName)
	fmt.Fprintf(&buf, "\r\n")
	buf.Write(attachData)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)

	return buf.Bytes()
}

func makeMetaSubject(path string, t time.Time) string {
	meta := emailMeta{Path: path, LMT: formatModTime(t)}
	b, _ := json.Marshal(meta)
	return string(b)
}

// ─── Тесты ──────────────────────────────────────────────────

func TestEmailName(t *testing.T) {
	c, _ := NewEmailClient(map[string]string{
		"imap": "imap.example.com:993",
		"smtp": "smtp.example.com:587",
		"user": "test@example.com",
		"pass": "secret",
	})
	if c.Name() != "email" {
		t.Errorf("expected 'email', got %q", c.Name())
	}
}

func TestEmailNewRequiredFields(t *testing.T) {
	_, err := NewEmailClient(map[string]string{})
	if err == nil {
		t.Error("expected error for empty config")
	}
	_, err = NewEmailClient(map[string]string{
		"imap": "imap:993",
		"smtp": "smtp:587",
		"user": "user",
	})
	if err == nil {
		t.Error("expected error without pass")
	}
}

func TestEmailNewDefaults(t *testing.T) {
	c, err := NewEmailClient(map[string]string{
		"imap": "imap:993",
		"smtp": "smtp:587",
		"user": "u",
		"pass": "p",
	})
	if err != nil {
		t.Fatalf("NewEmailClient: %v", err)
	}
	if c.folder != "INBOX" {
		t.Errorf("expected folder INBOX, got %s", c.folder)
	}
	if !c.compress {
		t.Error("expected compress=true by default")
	}
}

func TestEmailParseMeta(t *testing.T) {
	subject := `{"path":"subdir/photo.jpg","lmt":"2026-07-09 14:30:00"}`
	meta, err := parseEmailMeta(subject)
	if err != nil {
		t.Fatalf("parseEmailMeta: %v", err)
	}
	if meta.Path != "subdir/photo.jpg" {
		t.Errorf("expected path 'subdir/photo.jpg', got %q", meta.Path)
	}
	if meta.LMT != "2026-07-09 14:30:00" {
		t.Errorf("expected lmt '2026-07-09 14:30:00', got %q", meta.LMT)
	}
}

func TestEmailParseMetaInvalid(t *testing.T) {
	_, err := parseEmailMeta("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	_, err = parseEmailMeta(`{"path":"","lmt":""}`)
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestEmailGzipRoundtrip(t *testing.T) {
	original := []byte("hello world, this is test data for gzip roundtrip")
	compressed, err := gzipData(original)
	if err != nil {
		t.Fatalf("gzipData: %v", err)
	}
	uncompressed, err := gunzipData(compressed)
	if err != nil {
		t.Fatalf("gunzipData: %v", err)
	}
	if !bytes.Equal(original, uncompressed) {
		t.Errorf("gzip roundtrip failed")
	}
}

func TestEmailList(t *testing.T) {
	now := time.Date(2026, 7, 9, 14, 30, 0, 0, time.UTC)
	msg1 := buildEmailMessage(makeMetaSubject("file1.txt", now), []byte("data1"), false)
	msg2 := buildEmailMessage(makeMetaSubject("sub/file2.jpg", now), []byte("data2"), false)

	imap := newMockIMAPServer(t, []mockIMAPMsg{
		{Subject: makeMetaSubject("file1.txt", now), Body: msg1},
		{Subject: makeMetaSubject("sub/file2.jpg", now), Body: msg2},
	})
	defer imap.close()

	smtpSrv := newMockSMTPServer(t)
	defer smtpSrv.close()

	client, _ := NewEmailClient(map[string]string{
		"imap":              imap.addr(),
		"smtp":              smtpSrv.addr(),
		"user":              "test@example.com",
		"pass":              "pass",
		"self_signed_certs": "true",
	})

	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	paths := map[string]bool{}
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["file1.txt"] {
		t.Error("expected file1.txt in list")
	}
	if !paths["sub/file2.jpg"] {
		t.Error("expected sub/file2.jpg in list")
	}
}

func TestEmailListEmpty(t *testing.T) {
	imap := newMockIMAPServer(t, []mockIMAPMsg{})
	defer imap.close()
	smtpSrv := newMockSMTPServer(t)
	defer smtpSrv.close()

	client, _ := NewEmailClient(map[string]string{
		"imap":              imap.addr(),
		"smtp":              smtpSrv.addr(),
		"user":              "t@t.com",
		"pass":              "p",
		"self_signed_certs": "true",
	})

	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestEmailSubjectParse(t *testing.T) {
	imap := newMockIMAPServer(t, []mockIMAPMsg{
		{Subject: "not json", Body: []byte("content")},
		{Subject: makeMetaSubject("valid.txt", time.Now()), Body: []byte("content")},
	})
	defer imap.close()
	smtpSrv := newMockSMTPServer(t)
	defer smtpSrv.close()

	client, _ := NewEmailClient(map[string]string{
		"imap":              imap.addr(),
		"smtp":              smtpSrv.addr(),
		"user":              "t@t.com",
		"pass":              "p",
		"self_signed_certs": "true",
	})

	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 valid file, got %d", len(files))
	}
	if files[0].Path != "valid.txt" {
		t.Errorf("expected 'valid.txt', got %q", files[0].Path)
	}
}

func TestEmailLargeFile(t *testing.T) {
	client, _ := NewEmailClient(map[string]string{
		"imap":     "imap:993",
		"smtp":     "smtp:587",
		"user":     "u",
		"pass":     "p",
		"max_size": "10",
	})

	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.bin")
	data := make([]byte, 100)
	os.WriteFile(largeFile, data, 0644)

	err := client.Push(largeFile, "large.bin")
	if err != ErrFileTooLarge {
		t.Errorf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestEmailDelete(t *testing.T) {
	now := time.Now()
	msg1 := buildEmailMessage(makeMetaSubject("delete_me.txt", now), []byte("data"), false)
	imap := newMockIMAPServer(t, []mockIMAPMsg{
		{Subject: makeMetaSubject("delete_me.txt", now), Body: msg1},
	})
	defer imap.close()
	smtpSrv := newMockSMTPServer(t)
	defer smtpSrv.close()

	client, _ := NewEmailClient(map[string]string{
		"imap":              imap.addr(),
		"smtp":              smtpSrv.addr(),
		"user":              "t@t.com",
		"pass":              "p",
		"self_signed_certs": "true",
	})

	err := client.Delete("delete_me.txt")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	err = client.Delete("nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEmailMultipleFiles(t *testing.T) {
	var msgs []mockIMAPMsg
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("file%d.txt", i+1)
		subj := makeMetaSubject(path, time.Now())
		body := buildEmailMessage(subj, []byte(fmt.Sprintf("data%d", i+1)), false)
		msgs = append(msgs, mockIMAPMsg{Subject: subj, Body: body})
	}

	imap := newMockIMAPServer(t, msgs)
	defer imap.close()
	smtpSrv := newMockSMTPServer(t)
	defer smtpSrv.close()

	client, _ := NewEmailClient(map[string]string{
		"imap":              imap.addr(),
		"smtp":              smtpSrv.addr(),
		"user":              "t@t.com",
		"pass":              "p",
		"self_signed_certs": "true",
	})

	files, err := client.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 5 {
		t.Errorf("expected 5 files, got %d", len(files))
	}
}

func TestEmailFormatModTime(t *testing.T) {
	tm := time.Date(2026, 7, 9, 14, 30, 0, 0, time.UTC)
	s := formatModTime(tm)
	if s != "2026-07-09 14:30:00" {
		t.Errorf("expected '2026-07-09 14:30:00', got %q", s)
	}
	parsed, err := parseModTime(s)
	if err != nil {
		t.Fatalf("parseModTime: %v", err)
	}
	if !parsed.Equal(tm) {
		t.Errorf("roundtrip failed: %v vs %v", tm, parsed)
	}
}

func TestEmailPushToList(t *testing.T) {
	smtpSrv := newMockSMTPServer(t)
	defer smtpSrv.close()

	imap := newMockIMAPServer(t, []mockIMAPMsg{})
	defer imap.close()

	client, _ := NewEmailClient(map[string]string{
		"imap":              imap.addr(),
		"smtp":              smtpSrv.addr(),
		"user":              "test@example.com",
		"pass":              "pass",
		"compress":          "false",
		"self_signed_certs": "true",
	})

	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(localFile, []byte("push data"), 0644)

	err := client.Push(localFile, "test.txt")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	msg := smtpSrv.lastMessage()
	if msg == nil {
		t.Fatal("expected SMTP to receive message")
	}
	if !bytes.Contains(msg, []byte("Subject:")) {
		t.Error("expected Subject in SMTP message")
	}
}

func TestEmailExtractAttachments(t *testing.T) {
	data := []byte("test attachment data")
	body := buildEmailMessage(makeMetaSubject("f.txt", time.Now()), data, false)

	attachments, err := extractAttachments(body)
	if err != nil {
		t.Fatalf("extractAttachments: %v", err)
	}
	if len(attachments) == 0 {
		t.Fatal("expected at least 1 attachment")
	}
	if !bytes.Contains(attachments[0], data) {
		t.Errorf("attachment should contain original data")
	}
}

func TestEmailExtractAttachmentsGzip(t *testing.T) {
	original := []byte("gzipped content")
	body := buildEmailMessage(makeMetaSubject("f.txt", time.Now()), original, true)

	attachments, err := extractAttachments(body)
	if err != nil {
		t.Fatalf("extractAttachments: %v", err)
	}
	if len(attachments) == 0 {
		t.Fatal("expected at least 1 attachment")
	}

	uncompressed, err := gunzipData(attachments[0])
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if !bytes.Equal(original, uncompressed) {
		t.Errorf("expected %q, got %q", original, uncompressed)
	}
}

func TestEmailIMAPConnection(t *testing.T) {
	imap := newMockIMAPServer(t, []mockIMAPMsg{
		{Subject: makeMetaSubject("test.txt", time.Now()), Body: []byte("test")},
	})
	defer imap.close()

	client, _ := NewEmailClient(map[string]string{
		"imap":              imap.addr(),
		"smtp":              imap.addr(),
		"user":              "user",
		"pass":              "pass",
		"self_signed_certs": "true",
	})

	conn, err := client.dialIMAP(imap.addr())
	if err != nil {
		t.Fatalf("dialIMAP: %v", err)
	}
	defer conn.close()

	if err := conn.login("user", "pass"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := conn.selectMailbox("INBOX"); err != nil {
		t.Fatalf("select: %v", err)
	}
	ids, err := conn.search("ALL")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 message, got %d", len(ids))
	}
}

func TestEmailSMTPConnection(t *testing.T) {
	srv := newMockSMTPServer(t)
	defer srv.close()

	host := strings.Split(srv.addr(), ":")[0]
	auth := smtp.PlainAuth("", "user", "pass", host)
	err := smtp.SendMail(srv.addr(), auth, "from@test.com", []string{"to@test.com"}, []byte("Subject: test\r\n\r\nbody"))
	if err != nil {
		t.Fatalf("SendMail: %v", err)
	}

	msg := srv.lastMessage()
	if msg == nil {
		t.Fatal("expected message on SMTP server")
	}
	if !bytes.Contains(msg, []byte("Subject: test")) {
		t.Errorf("expected Subject in message")
	}
}
