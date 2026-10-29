package front

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"onionmail/internal/omcore"
)

type pop3Conn struct {
	c       net.Conn
	r       *bufio.Reader
	w       *bufio.Writer
	timeout time.Duration
}

func (p *pop3Conn) touch() {
	if p.timeout <= 0 {
		return
	}
	_ = p.c.SetDeadline(time.Now().Add(p.timeout))
}

func loopbackTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}
}

func dialPOP3(user, pass string, timeout time.Duration) (*pop3Conn, error) {
	if strings.ContainsAny(user, "\r\n\x00") || strings.ContainsAny(pass, "\r\n\x00") {
		return nil, fmt.Errorf("invalid POP3 credentials")
	}
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", pop3Port), timeout)
	if err != nil {
		return nil, err
	}
	p := &pop3Conn{c: c, r: bufio.NewReader(c), w: bufio.NewWriter(c), timeout: timeout}
	if _, err := p.line(); err != nil {
		c.Close()
		return nil, err
	}
	if err := p.cmd("STLS"); err != nil {
		c.Close()
		return nil, err
	}
	tc := tls.Client(c, loopbackTLSConfig())
	if err := tc.Handshake(); err != nil {
		c.Close()
		return nil, err
	}
	p.c, p.r, p.w = tc, bufio.NewReader(tc), bufio.NewWriter(tc)
	if err := p.cmd("USER " + user); err != nil {
		p.quit()
		return nil, err
	}
	if err := p.cmd("PASS " + pass); err != nil {
		p.quit()
		return nil, err
	}
	return p, nil
}

var pop3Port = 20110
var smtpPort = 20025

func (p *pop3Conn) line() (string, error) {
	p.touch()
	s, err := p.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

func (p *pop3Conn) cmd(s string) error {
	p.touch()
	if _, err := p.w.WriteString(s + "\r\n"); err != nil {
		return err
	}
	if err := p.w.Flush(); err != nil {
		return err
	}
	line, err := p.line()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "+OK") {
		return fmt.Errorf("%s", line)
	}
	return nil
}

func (p *pop3Conn) multi(s string) ([]string, error) {
	return p.multiLimited(s, 0)
}

func (p *pop3Conn) multiLimited(s string, maxBytes int64) ([]string, error) {
	p.touch()
	if _, err := p.w.WriteString(s + "\r\n"); err != nil {
		return nil, err
	}
	if err := p.w.Flush(); err != nil {
		return nil, err
	}
	line, err := p.line()
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "+OK") {
		return nil, fmt.Errorf("%s", line)
	}
	var out []string
	var total int64
	for {
		l, err := p.line()
		if err != nil {
			return nil, err
		}
		if l == "." {
			break
		}
		if strings.HasPrefix(l, "..") {
			l = l[1:]
		}
		total += int64(len(l)) + 2
		if maxBytes > 0 && total > maxBytes {
			return nil, fmt.Errorf("message exceeds size limit")
		}
		out = append(out, l)
	}
	return out, nil
}

func (p *pop3Conn) stat() (int, int, error) {
	p.touch()
	if _, err := p.w.WriteString("STAT\r\n"); err != nil {
		return 0, 0, err
	}
	if err := p.w.Flush(); err != nil {
		return 0, 0, err
	}
	line, err := p.line()
	if err != nil {
		return 0, 0, err
	}
	f := strings.Fields(line)
	if len(f) < 3 || !strings.HasPrefix(line, "+OK") {
		return 0, 0, fmt.Errorf("%s", line)
	}
	n, _ := strconv.Atoi(f[1])
	size, _ := strconv.Atoi(f[2])
	return n, size, nil
}

func (p *pop3Conn) quit() {
	if p == nil {
		return
	}
	_, _ = p.w.WriteString("QUIT\r\n")
	_ = p.w.Flush()
	_ = p.c.Close()
}

func pop3Stats(login, password string) (int, int, error) {
	p, err := dialPOP3(login, password, 8*time.Second)
	if err != nil {
		return 0, 0, err
	}
	defer p.quit()
	return p.stat()
}

func pop3Exists(login string) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", pop3Port), 5*time.Second)
	if err != nil {
		return false
	}
	p := &pop3Conn{c: c, r: bufio.NewReader(c), w: bufio.NewWriter(c), timeout: 5 * time.Second}
	defer p.quit()
	if _, err := p.line(); err != nil {
		return false
	}
	if err := p.cmd("STLS"); err != nil {
		return false
	}
	tc := tls.Client(c, loopbackTLSConfig())
	if err := tc.Handshake(); err != nil {
		return false
	}
	p.c, p.r, p.w = tc, bufio.NewReader(tc), bufio.NewWriter(tc)
	if err := p.cmd("USER " + login); err != nil {
		return false
	}
	return true
}

type smtpConn struct {
	c       net.Conn
	r       *bufio.Reader
	w       *bufio.Writer
	timeout time.Duration
}

func (s *smtpConn) touch() {
	if s.timeout <= 0 {
		return
	}
	_ = s.c.SetDeadline(time.Now().Add(s.timeout))
}

func dialSMTP(user, pass string, timeout time.Duration) (*smtpConn, error) {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", smtpPort), timeout)
	if err != nil {
		return nil, err
	}
	s := &smtpConn{c: c, r: bufio.NewReader(c), w: bufio.NewWriter(c), timeout: timeout}
	if _, err := s.response(); err != nil {
		c.Close()
		return nil, err
	}
	if _, err := s.ehlo(); err != nil {
		c.Close()
		return nil, err
	}
	if err := s.command("STARTTLS"); err != nil {
		c.Close()
		return nil, err
	}
	tc := tls.Client(c, loopbackTLSConfig())
	if err := tc.Handshake(); err != nil {
		c.Close()
		return nil, err
	}
	s.c, s.r, s.w = tc, bufio.NewReader(tc), bufio.NewWriter(tc)
	caps, err := s.ehlo()
	if err != nil {
		s.close()
		return nil, err
	}
	if err := s.auth(user, pass, caps); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *smtpConn) response() (string, error) {
	s.touch()
	var out strings.Builder
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return out.String(), err
		}
		out.WriteString(line)
		if len(line) < 4 || line[3] != '-' {
			break
		}
	}
	code := ""
	text := out.String()
	if len(text) >= 3 {
		code = text[:3]
	}
	if !strings.HasPrefix(code, "2") && !strings.HasPrefix(code, "3") {
		return text, fmt.Errorf("%s", strings.TrimSpace(text))
	}
	return text, nil
}

func (s *smtpConn) command(cmd string) error {
	s.touch()
	if _, err := s.w.WriteString(cmd + "\r\n"); err != nil {
		return err
	}
	if err := s.w.Flush(); err != nil {
		return err
	}
	_, err := s.response()
	return err
}

func (s *smtpConn) ehlo() (map[string]bool, error) {
	s.touch()
	if _, err := s.w.WriteString("EHLO localhost\r\n"); err != nil {
		return nil, err
	}
	if err := s.w.Flush(); err != nil {
		return nil, err
	}
	text, err := s.response()
	if err != nil {
		return nil, err
	}
	caps := map[string]bool{}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if len(l) > 4 {
			l = l[4:]
		}
		up := strings.ToUpper(l)
		if i := strings.IndexByte(up, ' '); i >= 0 {
			up = up[:i]
		}
		if up != "" {
			caps[up] = true
		}
	}
	return caps, nil
}

func (s *smtpConn) auth(user, pass string, caps map[string]bool) error {
	s.touch()
	if caps["PLAIN"] || !caps["LOGIN"] {
		token := base64.StdEncoding.EncodeToString([]byte("\x00" + user + "\x00" + pass))
		if _, err := s.w.WriteString("AUTH PLAIN " + token + "\r\n"); err == nil {
			if err = s.w.Flush(); err == nil {
				if _, e := s.response(); e == nil {
					return nil
				}
			}
		}
	}
	if err := s.command("AUTH LOGIN"); err != nil {
		return err
	}
	if err := s.command(base64.StdEncoding.EncodeToString([]byte(user))); err != nil {
		return err
	}
	return s.command(base64.StdEncoding.EncodeToString([]byte(pass)))
}

type outboundAttachment struct {
	Name        string
	ContentType string
	Reader      io.Reader
}

type smtpDataWriter struct {
	w           *bufio.Writer
	atLineStart bool
}

func (d *smtpDataWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if d.atLineStart && b == '.' {
			if err := d.w.WriteByte('.'); err != nil {
				return 0, err
			}
		}
		if err := d.w.WriteByte(b); err != nil {
			return 0, err
		}
		d.atLineStart = b == '\n'
	}
	return len(p), nil
}

func (d *smtpDataWriter) WriteString(v string) (int, error) {
	return d.Write([]byte(v))
}

type base64LineWriter struct {
	w     io.Writer
	n     int
	dirty bool
}

func (l *base64LineWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		space := 76 - l.n
		if space > len(p) {
			space = len(p)
		}
		if _, err := l.w.Write(p[:space]); err != nil {
			return written, err
		}
		written += space
		l.n += space
		l.dirty = true
		p = p[space:]
		if l.n == 76 {
			if _, err := l.w.Write([]byte("\r\n")); err != nil {
				return written, err
			}
			l.n = 0
			l.dirty = false
		}
	}
	return written, nil
}

func (l *base64LineWriter) Close() error {
	if !l.dirty {
		return nil
	}
	l.n = 0
	l.dirty = false
	_, err := l.w.Write([]byte("\r\n"))
	return err
}

func writeBase64Part(w io.Writer, r io.Reader) error {
	lw := &base64LineWriter{w: w}
	enc := base64.NewEncoder(base64.StdEncoding, lw)
	_, err := io.Copy(enc, r)
	if e := enc.Close(); err == nil {
		err = e
	}
	if e := lw.Close(); err == nil {
		err = e
	}
	return err
}

func (s *smtpConn) sendWithAttachment(domain, from, to, subject, body string, att *outboundAttachment) error {
	if err := s.command("MAIL FROM:<" + from + ">"); err != nil {
		return err
	}
	rcpts := []string{to}
	if list, err := mail.ParseAddressList(to); err == nil && len(list) > 0 {
		rcpts = rcpts[:0]
		for _, a := range list {
			rcpts = append(rcpts, a.Address)
		}
	}
	for _, rcpt := range rcpts {
		if err := s.command("RCPT TO:<" + rcpt + ">"); err != nil {
			return err
		}
	}
	if err := s.command("DATA"); err != nil {
		return err
	}
	if s.timeout > 0 {
		_ = s.c.SetDeadline(time.Now().Add(5 * time.Minute))
	}
	date := time.Now().Format(time.RFC1123Z)
	mid := fmt.Sprintf("<%s@%s>", randomToken(18), domain)
	encSubject := mime.QEncoding.Encode("utf-8", subject)
	dw := &smtpDataWriter{w: s.w, atLineStart: true}
	boundary := "----=_Part_" + randomToken(24)
	fmt.Fprintf(dw, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\n", from, to, encSubject, date, mid)
	if att == nil {
		fmt.Fprint(dw, "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		if err := writeBase64Part(dw, strings.NewReader(body)); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(dw, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary)
		fmt.Fprintf(dw, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", boundary)
		if err := writeBase64Part(dw, strings.NewReader(body)); err != nil {
			return err
		}
		name := safeFilename(att.Name)
		ctype := att.ContentType
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		ctypeHeader := mime.FormatMediaType(ctype, map[string]string{"name": name})
		if ctypeHeader == "" {
			ctypeHeader = "application/octet-stream"
		}
		fmt.Fprintf(dw, "--%s\r\nContent-Type: %s\r\nContent-Disposition: %s\r\nContent-Transfer-Encoding: base64\r\n\r\n",
			boundary, ctypeHeader, mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		if err := writeBase64Part(dw, att.Reader); err != nil {
			return err
		}
		fmt.Fprintf(dw, "--%s--\r\n", boundary)
	}
	if _, err := s.w.WriteString(".\r\n"); err != nil {
		return err
	}
	if err := s.w.Flush(); err != nil {
		return err
	}
	_, err := s.response()
	return err
}

func (s *smtpConn) quit() {
	if s == nil {
		return
	}
	_, _ = s.w.WriteString("QUIT\r\n")
	_ = s.w.Flush()
	_ = s.c.Close()
}

func (s *smtpConn) close() { _ = s.c.Close() }

func (s *Server) smtpSendAttachment(user, pass, from, to, subject, body string, att *outboundAttachment) error {
	c, err := dialSMTP(user, pass, 30*time.Second)
	if err != nil {
		return err
	}
	defer c.quit()
	return c.sendWithAttachment(s.Onion, from, to, subject, body, att)
}

func (s *Server) inboxItems(user, password string, limit int) ([]map[string]any, error) {
	p, err := dialPOP3(user, password, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer p.quit()
	p.w.WriteString("LIST\r\n")
	p.w.Flush()
	status, err := p.line()
	if err != nil || !strings.HasPrefix(status, "+OK") {
		if err == nil {
			err = fmt.Errorf("%s", status)
		}
		return nil, err
	}
	type pair struct{ n, size int }
	var nums []pair
	for {
		l, err := p.line()
		if err != nil {
			return nil, err
		}
		if l == "." {
			break
		}
		f := strings.Fields(l)
		if len(f) >= 2 {
			n, _ := strconv.Atoi(f[0])
			sz, _ := strconv.Atoi(f[1])
			nums = append(nums, pair{n, sz})
		}
	}
	if limit > 0 && len(nums) > limit {
		nums = nums[len(nums)-limit:]
	}
	var items []map[string]any
	for i := len(nums) - 1; i >= 0; i-- {
		lines, err := p.multi(fmt.Sprintf("TOP %d 120", nums[i].n))
		if err != nil {
			continue
		}
		msg, err := mail.ReadMessage(bytes.NewReader([]byte(strings.Join(lines, "\r\n") + "\r\n")))
		if err != nil {
			continue
		}
		rawBody, _ := io.ReadAll(io.LimitReader(msg.Body, 256*1024))
		rawBody = decodeTransfer(msg.Header.Get("Content-Transfer-Encoding"), rawBody)
		pgp := detectPGPText(string(rawBody))
		if pgp == "" {
			pgp = detectPGPHeader(msg.Header.Get("Content-Type"))
		}
		items = append(items, map[string]any{
			"n":       nums[i].n,
			"size":    nums[i].size,
			"from":    formatAddr(msg.Header.Get("From")),
			"subject": headerOr(msg.Header.Get("Subject"), "(no subject)"),
			"date":    decodeHeader(msg.Header.Get("Date")),
			"pgp":     pgp,
		})
	}
	return items, nil
}

type emailAttachment struct {
	Name        string
	ContentType string
	Data        []byte
}

func safeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '"' {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

func (s *Server) sendDownload(w http.ResponseWriter, name, ctype string, data []byte) {
	name = safeFilename(name)
	mt := "application/octet-stream"
	if ctype != "" {
		if parsed, _, err := mime.ParseMediaType(ctype); err == nil {
			mt = parsed
		}
	}
	w.Header().Set("Content-Type", mt)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) fetchRawMessage(user, password string, n int) ([]byte, error) {
	p, err := dialPOP3(user, password, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer p.quit()
	lines, err := p.multiLimited(fmt.Sprintf("RETR %d", n), s.maxMessageBytes())
	if err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\r\n") + "\r\n"), nil
}

func (s *Server) fetchMessage(user, password string, n int, emptyText, truncatedText string) (map[string]any, error) {
	raw, err := s.fetchRawMessage(user, password, n)
	if err != nil {
		return nil, err
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	body, atts := extractBody(msg, emptyText, truncatedText)
	pgp := detectPGPText(body)
	if pgp == "" {
		pgp = detectPGPHeader(msg.Header.Get("Content-Type"))
	}
	return map[string]any{
		"from":        formatAddr(msg.Header.Get("From")),
		"to":          formatAddr(msg.Header.Get("To")),
		"subject":     headerOr(msg.Header.Get("Subject"), "(no subject)"),
		"date":        decodeHeader(msg.Header.Get("Date")),
		"body":        body,
		"attachments": atts,
		"pgp":         pgp,
	}, nil
}

func detectPGPText(v string) string {
	switch {
	case strings.Contains(v, "-----BEGIN PGP PUBLIC KEY BLOCK-----"):
		return "keys"
	case strings.Contains(v, "-----BEGIN PGP MESSAGE-----"):
		return "encrypted"
	case strings.Contains(v, "-----BEGIN PGP SIGNATURE-----"), strings.Contains(v, "-----BEGIN PGP SIGNED MESSAGE-----"):
		return "signed"
	}
	return ""
}

func detectPGPHeader(ctype string) string {
	ctype = strings.ToLower(ctype)
	switch {
	case strings.Contains(ctype, "application/pgp-keys"):
		return "keys"
	case strings.Contains(ctype, "multipart/encrypted"), strings.Contains(ctype, "application/pgp-encrypted"):
		return "encrypted"
	case strings.Contains(ctype, "multipart/signed"), strings.Contains(ctype, "application/pgp-signature"):
		return "signed"
	}
	return ""
}

func extractAttachment(raw []byte, idx int) (emailAttachment, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return emailAttachment{}, err
	}
	var found emailAttachment
	ok := false
	count := 0
	parts := 0
	var walk func(h mail.Header, body io.Reader, depth int)
	walk = func(h mail.Header, body io.Reader, depth int) {
		if depth > 20 || ok {
			return
		}
		parts++
		if parts > maxMimeParts {
			return
		}
		mt, params := partMediaType(h)
		disp := strings.ToLower(h.Get("Content-Disposition"))
		filename := partFilename(h, params)
		if isAttachmentPart(mt, disp, filename) {
			if count == idx {
				data, _ := io.ReadAll(io.LimitReader(body, 50_000_000))
				data = decodeTransfer(h.Get("Content-Transfer-Encoding"), data)
				found = emailAttachment{Name: attachmentName(filename, count), ContentType: mt, Data: data}
				ok = true
				return
			}
			count++
			return
		}
		if strings.HasPrefix(mt, "multipart/") {
			boundary := params["boundary"]
			if boundary == "" {
				return
			}
			mr := multipart.NewReader(body, boundary)
			for !ok {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				walk(mail.Header(part.Header), part, depth+1)
			}
		}
	}
	walk(msg.Header, msg.Body, 0)
	if !ok {
		return emailAttachment{}, fmt.Errorf("attachment not found")
	}
	return found, nil
}

func partMediaType(h mail.Header) (string, map[string]string) {
	ctype := h.Get("Content-Type")
	if ctype == "" {
		ctype = "text/plain"
	}
	mt, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		return "text/plain", map[string]string{}
	}
	return mt, params
}

func partFilename(h mail.Header, params map[string]string) string {
	filename := ""
	if _, dp, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil {
		filename = dp["filename"]
	}
	if filename == "" {
		filename = params["name"]
	}
	return decodeHeader(filename)
}

func attachmentName(filename string, idx int) string {
	name := strings.TrimSpace(filename)
	if name == "" {
		name = fmt.Sprintf("attachment-%d", idx+1)
	}
	return name
}

func isAttachmentPart(mt, disp, filename string) bool {
	return filename != "" || strings.Contains(disp, "attachment")
}

func passwordVariants(p string) []string {
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	add := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add(p)
	add(strings.ReplaceAll(strings.TrimSpace(p), " ", "+"))
	return out
}

func dialPOP3Any(user, pass string) (*pop3Conn, string, error) {
	var lastErr error
	for _, candidate := range passwordVariants(pass) {
		p, err := dialPOP3(user, candidate, 30*time.Second)
		if err == nil {
			return p, candidate, nil
		}
		lastErr = err
	}
	return nil, pass, lastErr
}

func dialSMTPAny(user, pass string) (*smtpConn, string, error) {
	var lastErr error
	for _, candidate := range passwordVariants(pass) {
		c, err := dialSMTP(user, candidate, 30*time.Second)
		if err == nil {
			return c, candidate, nil
		}
		lastErr = err
	}
	return nil, pass, lastErr
}

var wordDecoder = &mime.WordDecoder{CharsetReader: charsetReader}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	cs := strings.ToLower(strings.TrimSpace(charset))
	switch cs {
	case "utf-8", "utf8", "us-ascii", "ascii", "":
		return input, nil
	case "iso-8859-1", "latin1", "latin-1", "windows-1252", "cp1252":
		b, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		return strings.NewReader(string(r)), nil
	}
	return input, nil
}

func decodeHeader(v string) string {
	if v == "" {
		return ""
	}
	if out, err := wordDecoder.DecodeHeader(v); err == nil {
		return out
	}
	return v
}

func headerOr(v, def string) string {
	out := decodeHeader(v)
	if strings.TrimSpace(out) == "" {
		return def
	}
	return out
}

func formatAddr(v string) string {
	if v == "" {
		return ""
	}
	list, err := mail.ParseAddressList(v)
	if err != nil {
		return v
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		name := decodeHeader(a.Name)
		if name != "" {
			out = append(out, name+" <"+a.Address+">")
		} else {
			out = append(out, a.Address)
		}
	}
	return strings.Join(out, ", ")
}

func humanSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

func htmlToText(data string) string {
	reScript := regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</\1>`)
	data = reScript.ReplaceAllString(data, "")
	reBlock := regexp.MustCompile(`(?i)<(br|p|div|tr|li|h[1-6]|blockquote)[^>]*>`)
	data = reBlock.ReplaceAllString(data, "\n")
	reEnd := regexp.MustCompile(`(?i)</(p|div|tr|li|h[1-6]|blockquote)>`)
	data = reEnd.ReplaceAllString(data, "\n")
	reTag := regexp.MustCompile(`(?s)<[^>]+>`)
	data = reTag.ReplaceAllString(data, "")
	data = html.UnescapeString(data)
	reNL := regexp.MustCompile(`\n{3,}`)
	data = reNL.ReplaceAllString(data, "\n\n")
	return strings.TrimSpace(data)
}

func extractBody(msg *mail.Message, emptyText, truncatedText string) (string, []emailAttachment) {
	var texts, htmls []string
	var atts []emailAttachment
	parts := 0
	var total int64
	var walk func(h mail.Header, body io.Reader, depth int)
	walk = func(h mail.Header, body io.Reader, depth int) {
		if depth > 20 {
			return
		}
		parts++
		if parts > maxMimeParts || total >= maxMimeTotalBytes {
			return
		}
		mt, params := partMediaType(h)
		disp := strings.ToLower(h.Get("Content-Disposition"))
		filename := partFilename(h, params)
		if isAttachmentPart(mt, disp, filename) {
			atts = append(atts, emailAttachment{Name: attachmentName(filename, len(atts)), ContentType: mt})
			return
		}
		if strings.HasPrefix(mt, "multipart/") {
			boundary := params["boundary"]
			if boundary == "" {
				return
			}
			mr := multipart.NewReader(body, boundary)
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				walk(mail.Header(part.Header), part, depth+1)
			}
			return
		}
		budget := int64(maxMimeTotalBytes) - total
		if budget > maxMimePartBytes {
			budget = maxMimePartBytes
		}
		raw, _ := io.ReadAll(io.LimitReader(body, budget))
		total += int64(len(raw))
		dec := decodeTransfer(h.Get("Content-Transfer-Encoding"), raw)
		if charset := params["charset"]; charset != "" {
			if r, err := charsetReader(charset, bytes.NewReader(dec)); err == nil {
				dec, _ = io.ReadAll(r)
			}
		}
		switch mt {
		case "text/plain":
			texts = append(texts, string(dec))
		case "text/html":
			htmls = append(htmls, string(dec))
		}
	}
	walk(msg.Header, msg.Body, 0)
	var nonEmpty []string
	for _, t := range texts {
		if strings.TrimSpace(t) != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}
	body := strings.Join(nonEmpty, "\n\n")
	if strings.TrimSpace(body) == "" && len(htmls) > 0 {
		var parts []string
		for _, h := range htmls {
			parts = append(parts, htmlToText(h))
		}
		body = strings.Join(parts, "\n\n")
	}
	if strings.TrimSpace(body) == "" {
		body = emptyText
	}
	if len(body) > MaxBody {
		cut := MaxBody
		for cut > 0 && !utf8.RuneStart(body[cut]) {
			cut--
		}
		body = body[:cut] + "\n\n" + truncatedText
	}
	return body, atts
}

const (
	maxMimeParts      = 200
	maxMimePartBytes  = 10 << 20
	maxMimeTotalBytes = 20 << 20
)

func decodeTransfer(enc string, raw []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "base64":
		out, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(raw)))
		if err == nil {
			return out
		}
	case "quoted-printable":
		out, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))
		if err == nil {
			return out
		}
	}
	return raw
}

func (s *Server) newSession(user, smtpPass, pop3Pass string) string {
	token := randomToken(32)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = &Session{User: user, SMTP: smtpPass, POP3: pop3Pass, CSRF: randomToken(24), Exp: time.Now().Add(time.Duration(SessionTTL) * time.Second)}
	now := time.Now()
	for k, v := range s.sessions {
		if v.Exp.Before(now) {
			delete(s.sessions, k)
		}
	}
	return token
}

func (s *Server) getSession(r *http.Request) (string, *Session) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[c.Value]
	if sess != nil && sess.Exp.After(time.Now()) {
		sess.Exp = time.Now().Add(time.Duration(SessionTTL) * time.Second)
		return c.Value, sess
	}
	delete(s.sessions, c.Value)
	return "", nil
}

func (s *Server) dropSession(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func sessionCookie(token string, secure bool) [2]string {
	sec := ""
	if secure {
		sec = "; Secure"
	}
	return [2]string{"Set-Cookie", fmt.Sprintf("%s=%s; Path=/web; HttpOnly; SameSite=Strict%s; Max-Age=%d", CookieName, token, sec, SessionTTL)}
}

func clearCookie(secure bool) [2]string {
	sec := ""
	if secure {
		sec = "; Secure"
	}
	return [2]string{"Set-Cookie", CookieName + "=deleted; Path=/web; HttpOnly; SameSite=Strict" + sec + "; Max-Age=0"}
}

func (s *Server) cookieSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

var userRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$`)

func sentPath(user string) (string, error) {
	if !userRe.MatchString(user) {
		return "", fmt.Errorf("bad user")
	}
	return filepath.Join("/var/lib/onionmail/webmail/sent", user+".json"), nil
}

func (s *Server) loadSent(user string) []map[string]any {
	path, err := sentPath(user)
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var data []map[string]any
	if json.Unmarshal(b, &data) != nil {
		return nil
	}
	return data
}

func (s *Server) addSent(user string, entry map[string]any) {
	path, err := sentPath(user)
	if err != nil {
		return
	}
	if err := os.MkdirAll(s.SentDir, 0o700); err != nil {
		return
	}
	s.sentMu.Lock()
	defer s.sentMu.Unlock()
	data := s.loadSent(user)
	data = append([]map[string]any{entry}, data...)
	if len(data) > 500 {
		data = data[:500]
	}
	b, _ := json.Marshal(data)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Chmod(tmp, 0o600)
		_ = os.Rename(tmp, path)
	}
}

type registryData struct {
	Users map[string]map[string]any `json:"users"`
}

func (s *Server) loadRegistry() registryData {
	b, err := os.ReadFile(s.Registry)
	if err != nil {
		return registryData{Users: map[string]map[string]any{}}
	}
	var data registryData
	if json.Unmarshal(b, &data) != nil || data.Users == nil {
		data.Users = map[string]map[string]any{}
	}
	return data
}

func (s *Server) saveRegistry(data registryData) error {
	if err := os.MkdirAll(filepath.Dir(s.Registry), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Registry + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o600)
	return os.Rename(tmp, s.Registry)
}

func (s *Server) clearUserExit(login string) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	data := s.loadRegistry()
	if entry := data.Users[login]; entry != nil {
		delete(entry, "exit")
		_ = s.saveRegistry(data)
	}
}

func (s *Server) registerUser(login, smtp, pop3, exitlogin string, lastLogin int64) bool {
	login = strings.ToLower(strings.TrimSpace(login))
	if !userRe.MatchString(login) {
		return false
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()
	data := s.loadRegistry()
	entry := data.Users[login]
	if entry == nil {
		entry = map[string]any{}
	}
	if smtp != "" {
		s.storeCredential(entry, "smtp", smtp)
	}
	if pop3 != "" {
		s.storeCredential(entry, "pop3", pop3)
	}
	if exitlogin != "" {
		entry["exit"] = strings.ToLower(strings.TrimSpace(exitlogin))
	}
	if lastLogin > 0 {
		entry["last_login"] = lastLogin
	}
	if _, ok := entry["created"]; !ok {
		entry["created"] = time.Now().Unix()
	}
	data.Users[login] = entry
	_ = s.saveRegistry(data)
	return true
}

func (s *Server) webmailBlocked(login string) bool {
	login = strings.ToLower(strings.TrimSpace(login))
	if !userRe.MatchString(login) {
		return false
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()
	data := s.loadRegistry()
	entry := data.Users[login]
	return entry != nil && omcoreBoolDefault(entry, "webmail_blocked", false)
}

func (s *Server) setWebmailBlocked(login string, blocked bool) {
	login = strings.ToLower(strings.TrimSpace(login))
	if !userRe.MatchString(login) {
		return
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()
	data := s.loadRegistry()
	entry := data.Users[login]
	if entry == nil {
		if !blocked {
			return
		}
		entry = map[string]any{}
	}
	if blocked {
		entry["webmail_blocked"] = true
	} else {
		delete(entry, "webmail_blocked")
	}
	data.Users[login] = entry
	_ = s.saveRegistry(data)
}

func (s *Server) controlStat(timeout time.Duration) map[string]any {
	pass := ""
	for _, line := range strings.Split(omcore.Read(s.ServerDir), "\n") {
		p := strings.Fields(line)
		if len(p) >= 2 && strings.EqualFold(p[0], "passwd") {
			pass = p[1]
			break
		}
	}
	port := omcore.ControlPort()
	if pass == "" {
		return map[string]any{}
	}
	c, err := omcore.DialControl(port, timeout)
	if err != nil {
		return map[string]any{}
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	r := bufio.NewReader(c)
	readline := func() (string, error) {
		l, err := r.ReadString('\n')
		return strings.TrimRight(l, "\r\n"), err
	}
	if _, err := readline(); err != nil {
		return map[string]any{}
	}
	fmt.Fprintf(c, "server %s %s\n", s.Nick, pass)
	auth, err := readline()
	if err != nil || !strings.HasPrefix(auth, "+OK") {
		return map[string]any{}
	}
	fmt.Fprint(c, "stat\n")
	if line, err := readline(); err != nil || !strings.HasPrefix(line, "+OK") {
		return map[string]any{}
	}
	out := map[string]any{}
	for {
		line, err := readline()
		if err != nil || line == "." || line == "" || strings.HasPrefix(line, "-ERR") {
			break
		}
		if i := strings.IndexByte(line, ':'); i >= 0 {
			out[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
		}
	}
	fmt.Fprint(c, "quit\n")
	return out
}

func (s *Server) controlVer() string {
	reply, err := s.controlCommand("ver", 10*time.Second)
	if err == nil && strings.HasPrefix(reply, "+OK ") {
		f := strings.Fields(reply[4:])
		if len(f) > 0 {
			return f[0]
		}
	}
	return ""
}

func processUptime() int {
	st, err := os.Stat("/proc/1")
	if err != nil {
		return 0
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		return int(time.Since(time.Unix(int64(sys.Ctim.Sec), int64(sys.Ctim.Nsec))).Seconds())
	}
	return int(time.Since(st.ModTime()).Seconds())
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink == 0 {
			total += info.Size()
		}
		return nil
	})
	return total
}

func fmtSize(num int64) string {
	if num < 0 {
		num = 0
	}
	kb := float64(num) / 1024.0
	rep := func(v float64, unit string) string {
		return strings.ReplaceAll(fmt.Sprintf("%.1f", v), ".", ",") + " " + unit
	}
	if kb <= 4000 {
		return rep(kb, "KB")
	}
	mb := kb / 1024.0
	if mb <= 8000 {
		return rep(mb, "MB")
	}
	gb := mb / 1024.0
	if gb <= 999 {
		return rep(gb, "GB")
	}
	tb := gb / 1024.0
	if tb <= 999 {
		return strings.ReplaceAll(fmt.Sprintf("%.2f", tb), ".", ",") + " TB"
	}
	return strings.ReplaceAll(fmt.Sprintf("%.2f", tb/1024.0), ".", ",") + " PB"
}

func fmtUptime(sec int) string {
	if sec <= 0 {
		return "—"
	}
	days := sec / 86400
	rem := sec % 86400
	hours := rem / 3600
	mins := (rem % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func fmtInt(num int64) string {
	s := strconv.FormatInt(num, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, " ")
	if neg {
		out = "-" + out
	}
	return out
}

func parseSuccessCreds(text string) (string, string) {
	plain := html.UnescapeString(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, " "))
	sm := regexp.MustCompile(`(?i)SMTP\s+Password\s*:?\s*([^\s<]{8,})`).FindStringSubmatch(plain)
	po := regexp.MustCompile(`(?i)POP3\s+Password\s*:?\s*([^\s<]{8,})`).FindStringSubmatch(plain)
	if sm != nil && po != nil {
		return sm[1], po[1]
	}
	return "", ""
}

var aliasRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,30}[a-z0-9]$`)

func validExitLogin(name string) bool {
	if !aliasRe.MatchString(name) {
		return false
	}
	if strings.Contains(name, "..") || name == "server" {
		return false
	}
	for _, suf := range []string{".onion", ".sys", ".op", ".list"} {
		if strings.HasSuffix(name, suf) {
			return false
		}
	}
	return true
}

func (s *Server) registrySeed() error {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	data := s.loadRegistry()
	changed := false
	sysop := filepath.Join(s.MailDir, "sysop.txt")
	if b, err := os.ReadFile(sysop); err == nil {
		text := string(b)
		sm := regexp.MustCompile(`(?i)SMTP\s+Password\s*:?\s*(\S+)`).FindStringSubmatch(text)
		po := regexp.MustCompile(`(?i)POP3\s+Password\s*:?\s*(\S+)`).FindStringSubmatch(text)
		if sm != nil && po != nil {
			entry := data.Users["sysop"]
			if entry == nil {
				entry = map[string]any{}
			}
			if s.entryCredential(entry, "smtp") == "" || s.entryCredential(entry, "pop3") == "" {
				s.storeCredential(entry, "smtp", sm[1])
				s.storeCredential(entry, "pop3", po[1])
				if _, ok := entry["created"]; !ok {
					entry["created"] = time.Now().Unix()
				}
				data.Users["sysop"] = entry
				changed = true
			}
		}
	}
	if entries, err := os.ReadDir(s.SentDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			login := strings.ToLower(strings.TrimSuffix(name, ".json"))
			if userRe.MatchString(login) {
				if _, ok := data.Users[login]; !ok {
					data.Users[login] = map[string]any{"created": time.Now().Unix()}
					changed = true
				}
			}
		}
	}
	if _, ok := data.Users["root"]; !ok && pop3Exists("root") {
		data.Users["root"] = map[string]any{"created": time.Now().Unix()}
		changed = true
	}
	if changed {
		return s.saveRegistry(data)
	}
	return nil
}

func onionRandom() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
