package front

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"onionmail/internal/olang"
	"onionmail/internal/omcore"
)

const (
	LangCookie  = "omlang"
	ThemeCookie = "omtheme"
	CookieName  = "omweb"
	ExitDomain  = "onionmail.info"
	SessionTTL  = 3600
	MaxBody     = 200_000
)

type Server struct {
	Nick         string
	DisplayName  string
	Bind         string
	Port         string
	Upstream     string
	SMTPPort     int
	POP3Port     int
	ServerDir    string
	ControlConf  string
	SentDir      string
	MailDir      string
	Registry     string
	RegistryKey  string
	ThemeDir     string
	Onion        string
	DefaultTheme string
	Bundle       *olang.Bundle
	AdminToken   string
	Client       *http.Client
	TLS          *tls.Config
	Log          io.Writer

	mu       sync.Mutex
	sessions map[string]*Session
	regMu    sync.Mutex
	sentMu   sync.Mutex
	stateMu  sync.Mutex
	stateMod time.Time
	stateVal map[string]any

	httpMu                       sync.Mutex
	http2, http403, http4, http5 int64
	statsMu                      sync.Mutex

	loginMu       sync.Mutex
	loginFailures map[string][]time.Time
}

type Session struct {
	User string
	SMTP string
	POP3 string
	CSRF string
	Exp  time.Time
}

type reqCtx struct {
	s     *Server
	lang  string
	theme string
	path  string
}

func NewServer() (*Server, error) {
	nick := os.Getenv("ONIONMAIL_NICK")
	if nick == "" {
		nick = "onionmail"
	}
	display := strings.TrimSpace(os.Getenv("ONIONMAIL_DISPLAY_NAME"))
	if display == "" {
		display = "example"
	}
	bind := strings.TrimSpace(os.Getenv("WEB_BIND"))
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := os.Getenv("WEB_PORT")
	if port == "" {
		port = "18000"
	}
	b, err := olang.Load("")
	if err != nil {
		return nil, err
	}
	theme := strings.ToLower(strings.TrimSpace(os.Getenv("ONIONMAIL_THEME")))
	if theme == "" {
		theme = "light"
	}
	s := &Server{
		Nick: nick, DisplayName: display, Bind: bind, Port: port, Upstream: "127.0.0.1:18001",
		SMTPPort: 20025, POP3Port: 20110,
		ServerDir:    "/etc/onionmail/servers.d/1-" + nick + ".conf",
		ControlConf:  "/etc/onionmail/control.conf",
		SentDir:      "/var/lib/onionmail/webmail/sent",
		MailDir:      "/var/onionmail/" + nick,
		Registry:     "/var/lib/onionmail/webmail/registry.json",
		RegistryKey:  defaultRegistryKeyPath,
		ThemeDir:     envOr("ONIONMAIL_THEME_DIR", "/usr/local/share/onionmail/theme"),
		DefaultTheme: theme, Bundle: b,
		AdminToken: randomToken(24),
		Client: &http.Client{
			Timeout: 240 * time.Second,
			Transport: &http.Transport{
				DisableCompression: true,
				DisableKeepAlives:  true,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		TLS:      &tls.Config{InsecureSkipVerify: true},
		Log:      os.Stdout,
		sessions: map[string]*Session{},

		loginFailures: map[string][]time.Time{},
	}
	s.Onion, err = s.readOnion()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (s *Server) readOnion() (string, error) {
	f, err := os.Open(s.ServerDir)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	for _, line := range strings.Split(string(b), "\n") {
		p := strings.Fields(line)
		if len(p) >= 2 && strings.EqualFold(p[0], "onion") {
			return p[1], nil
		}
	}
	return "", fmt.Errorf("onion address not found")
}

func (s *Server) logf(format string, args ...any) {
	fmt.Fprintf(s.Log, "%s %s\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"), fmt.Sprintf(format, args...))
}

func (s *Server) Serve() error {
	if err := os.MkdirAll(s.SentDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(s.SentDir, 0o700); err != nil {
		return err
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(s.Log, "registry seed failed: %v\n", r)
			}
		}()
		if err := s.registrySeed(); err != nil {
			fmt.Fprintf(s.Log, "registry seed failed: %v\n", err)
		}
	}()
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(s.Log, "registry migration failed: %v\n", r)
			}
		}()
		if err := s.migrateRegistrySecrets(); err != nil {
			fmt.Fprintf(s.Log, "registry migration failed: %v\n", err)
		}
	}()
	addr := net.JoinHostPort(s.Bind, s.Port)
	fmt.Fprintf(s.Log, "front ready for %s (webmail /web, upstream %s)\n", s.Onion, s.Upstream)
	go s.startControlGate()
	go s.serverStatsLoop()
	srv := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       300 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &statusWriter{ResponseWriter: w}
	s.serve(rec, r)
	s.recordHTTP(r.URL.Path, rec.statusCode())
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) statusCode() int {
	if w.code == 0 {
		return 200
	}
	return w.code
}

func (s *Server) recordHTTP(path string, code int) {
	if path == "/healthz" || strings.HasPrefix(path, "/theme-assets/") {
		return
	}
	var which int
	switch {
	case code >= 200 && code < 300:
		which = 1
	case code == 403:
		which = 2
	case code >= 400 && code < 500:
		which = 3
	case code >= 500 && code < 600:
		which = 4
	default:
		return
	}
	s.httpMu.Lock()
	switch which {
	case 1:
		s.http2++
	case 2:
		s.http403++
	case 3:
		s.http4++
	case 4:
		s.http5++
	}
	s.httpMu.Unlock()
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	ctx := s.newCtx(r)
	path := r.URL.Path
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		switch {
		case path == "/lang":
			lang := r.URL.Query().Get("l")
			if !s.Bundle.HasLang(lang) {
				lang = "en"
			}
			next := safeNext(r.URL.Query().Get("next"))
			w.Header().Add("Set-Cookie", fmt.Sprintf("%s=%s; Path=/; HttpOnly; SameSite=Lax; Max-Age=31536000", LangCookie, lang))
			s.redirect(w, r, next, nil)
			return
		case path == "/theme":
			theme := strings.ToLower(r.URL.Query().Get("t"))
			if theme != "light" && theme != "dark" {
				theme = "light"
			}
			next := safeNext(r.URL.Query().Get("next"))
			w.Header().Add("Set-Cookie", fmt.Sprintf("%s=%s; Path=/; HttpOnly; SameSite=Lax; Max-Age=31536000", ThemeCookie, theme))
			s.redirect(w, r, themeBustURL(next, theme), nil)
			return
		case strings.HasPrefix(path, "/theme-assets/"):
			s.serveThemeAsset(ctx, w, r, path)
			return
		case path == "/healthz":
			s.sendBytes(w, r, 200, []byte(s.Onion), "text/plain", nil)
			return
		}
		if s.adminHidden() && (path == "/logon.html" || strings.HasPrefix(path, "/admin")) {
			s.sendHTML(w, r, 404, "<h1>404</h1>", nil)
			return
		}
		if !s.adminEnabled() && strings.HasPrefix(path, "/admin") {
			s.redirect(w, r, "/logon.html", nil)
			return
		}
		switch {
		case path == "/register":
			s.redirect(w, r, "/register.html", nil)
		case path == "/register.html":
			s.registerPage(ctx, w, r)
		case path == "/admin" || path == "/admin/" || path == "/admin/index.html" || path == "/admin/storage.html":
			s.adminDashboard(ctx, w, r)
		case path == "/admin/server.html":
			s.adminServer(ctx, w, r)
		case path == "/web" || path == "/web/":
			if _, sess := s.getSession(r); sess != nil {
				s.redirect(w, r, "/web/inbox", nil)
			} else {
				s.sendHTML(w, r, 200, s.loginPage(ctx, ""), nil)
			}
		case strings.HasPrefix(path, "/web/"):
			s.webGet(ctx, w, r, path)
		default:
			s.proxy(ctx, w, r)
		}
	case http.MethodPost:
		if path != "/logon.html" && path != "/web/login" && !sameOrigin(r) {
			s.logf("cross-origin POST rejected: %s", path)
			s.sendHTML(w, r, 403, s.smallPage(ctx, ctx.t("Access denied"), ctx.t("Cross-origin request rejected.")), nil)
			return
		}
		if s.adminHidden() && (path == "/logon.html" || strings.HasPrefix(path, "/admin")) {
			s.sendHTML(w, r, 404, "<h1>404</h1>", nil)
			return
		}
		if !s.adminEnabled() && (path == "/logon.html" || strings.HasPrefix(path, "/admin")) {
			if path == "/logon.html" {
				s.disabledLogon(ctx, w, r)
				return
			}
			s.redirect(w, r, "/logon.html", nil)
			return
		}
		switch {
		case path == "/newuser.html":
			s.registerPost(ctx, w, r)
		case path == "/admin/storage.html":
			s.adminDashboard(ctx, w, r)
		case path == "/logon.html":
			s.logonProxy(ctx, w, r)
		case strings.HasPrefix(path, "/web/"):
			s.webPost(ctx, w, r, path)
		default:
			s.proxy(ctx, w, r)
		}
	default:
		s.proxy(ctx, w, r)
	}
}

func themeBustURL(next, theme string) string {
	u, err := url.Parse(next)
	if err != nil {
		return next
	}
	q := u.Query()
	q.Set("_theme", theme)
	q.Set("_ts", strconv.FormatInt(time.Now().UnixNano(), 10))
	u.RawQuery = q.Encode()
	return u.RequestURI()
}

func safeNext(n string) string {
	if n == "" || len(n) > 2048 {
		return "/"
	}
	if !strings.HasPrefix(n, "/") || strings.HasPrefix(n, "//") {
		return "/"
	}
	for _, r := range n {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return "/"
		}
	}
	u, err := url.Parse(n)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" {
		return "/"
	}
	if strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	return n
}

func (s *Server) newCtx(r *http.Request) *reqCtx {
	lang := "en"
	theme := s.defaultTheme()
	if c, err := r.Cookie(LangCookie); err == nil && s.Bundle.HasLang(c.Value) {
		lang = c.Value
	}
	if c, err := r.Cookie(ThemeCookie); err == nil && (c.Value == "light" || c.Value == "dark") {
		theme = c.Value
	}
	return &reqCtx{s: s, lang: lang, theme: theme, path: r.URL.RequestURI()}
}

func (s *Server) defaultTheme() string {
	design := omcore.Obj(s.omState(), "design")
	v := strings.ToLower(omcore.Str(design, "theme"))
	if v == "light" || v == "dark" {
		return v
	}
	if s.DefaultTheme == "dark" {
		return "dark"
	}
	return "light"
}

func (c *reqCtx) t(key string) string { return c.s.Bundle.FrontText(c.lang, key) }
func (c *reqCtx) tf(key string, args map[string]string) string {
	return c.s.Bundle.FrontFormat(c.lang, key, args)
}

func (s *Server) sendBytes(w http.ResponseWriter, r *http.Request, code int, data []byte, ctype string, extra [][2]string) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src 'self' data:; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	for _, kv := range extra {
		w.Header().Add(kv[0], kv[1])
	}
	w.WriteHeader(code)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (s *Server) sendHTML(w http.ResponseWriter, r *http.Request, code int, text string, extra [][2]string) {
	s.sendBytes(w, r, code, []byte(text), "text/html; charset=utf-8", extra)
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, location string, extra [][2]string) {
	w.Header().Set("Location", location)
	for _, kv := range extra {
		w.Header().Add(kv[0], kv[1])
	}
	s.sendBytes(w, r, 302, nil, "text/plain", nil)
}

func (s *Server) serveThemeAsset(ctx *reqCtx, w http.ResponseWriter, r *http.Request, path string) {
	name := strings.TrimPrefix(path, "/theme-assets/")
	ctype := map[string]string{
		"CIEU_white.png": "image/png", "CIED_white.png": "image/png",
		"CIEU_black.png": "image/png", "CIED_black.png": "image/png",
		"sun.svg": "image/svg+xml", "moon.svg": "image/svg+xml",
	}[name]
	if ctype == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		s.sendHTML(w, r, 404, "<h1>404</h1>", nil)
		return
	}
	data, err := os.ReadFile(filepath.Join(s.ThemeDir, name))
	if err != nil {
		s.sendHTML(w, r, 404, "<h1>404</h1>", nil)
		return
	}
	s.sendBytes(w, r, 200, data, ctype, nil)
}

func (s *Server) readBody(r *http.Request, limit int64) []byte {
	if limit <= 0 {
		limit = 300_000
	}
	lr := io.LimitReader(r.Body, limit+1)
	data, _ := io.ReadAll(lr)
	if int64(len(data)) > limit {
		return []byte{}
	}
	return data
}

func sameOrigin(r *http.Request) bool {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" {
		u, err := url.Parse(ref)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	return true
}

func (s *Server) form(r *http.Request) map[string][]string {
	raw := s.readBody(r, 300_000)
	v, err := urlParseQuery(string(raw))
	if err != nil {
		return map[string][]string{}
	}
	return v
}

func (s *Server) adminEnabled() bool {
	return omcore.Bool(s.omState(), "admin_enabled", true)
}

func (s *Server) adminHidden() bool {
	return omcore.Bool(s.omState(), "admin_hidden", false)
}

func (s *Server) adminVisible() bool {
	return s.adminEnabled() && !s.adminHidden()
}

func (s *Server) registration() map[string]any {
	v, ok := s.omState()["registration"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return v
}

func (s *Server) omLimits() map[string]any {
	lim, _ := s.omState()["limits"].(map[string]any)
	if lim == nil {
		lim = map[string]any{}
	}
	return map[string]any{
		"maxmsgsize":  omcore.Int(lim, "max_message_mb", 256) * 1024,
		"boxsize":     omcore.Int(lim, "box_mb", 4096) * 1024,
		"maxmlsize":   omcore.Int(lim, "max_ml", 4096),
		"maxmultimsg": omcore.Int(lim, "max_rcpt", 24),
		"maxspam":     omcore.Int(lim, "max_spam", 196),
		"maxmsgxusr":  omcore.Int(lim, "max_messages", 256),
		"debug":       omcore.Bool(lim, "debug", false),
		"autodelete":  omcore.Bool(lim, "autodelete", false),
	}
}

func (s *Server) maxAttachmentBytes() int64 {
	kb := int64(omcore.Int(s.omLimits(), "maxmsgsize", 262144))
	max := kb * 1024
	limit := max*3/4 - 1024*1024
	if limit < 64*1024 {
		limit = 64 * 1024
	}
	return limit
}

func (s *Server) maxMessageBytes() int64 {
	mb := int64(omcore.Int(s.omLimits(), "max_message_mb", 256))
	if mb < 1 {
		mb = 1
	}
	return mb * 1024 * 1024
}

func (s *Server) omState() map[string]any {
	path := omcore.StatePath
	st, err := os.Stat(path)
	if err != nil {
		return map[string]any{}
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.stateVal != nil && st.ModTime().Equal(s.stateMod) {
		return s.stateVal
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil || v == nil {
		v = map[string]any{}
	}
	s.stateMod = st.ModTime()
	s.stateVal = v
	return v
}

func (s *Server) omStateSave(st map[string]any) {
	if err := omcore.StateSave(omcore.State(st)); err != nil {
		s.logf("state save failed: %v", err)
	}
}

func (s *Server) controlCommand(cmd string, timeout time.Duration) (string, error) {
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
		return "", fmt.Errorf("service password not found")
	}
	return omcore.ControlAs(s.Nick, pass, port, cmd, timeout)
}

func (s *Server) fetchUpstream(r *http.Request, body []byte, path, method string) (int, http.Header, []byte, error) {
	if path == "" {
		path = r.URL.RequestURI()
	}
	if method == "" {
		method = r.Method
	}
	req, err := http.NewRequest(method, "http://"+s.Upstream+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	if r.Host != "" {
		req.Host = r.Host
	}
	skip := map[string]bool{"connection": true, "keep-alive": true, "proxy-authenticate": true, "proxy-authorization": true, "te": true, "trailers": true, "transfer-encoding": true, "upgrade": true, "accept-encoding": true, "content-length": true}
	for k, vals := range r.Header {
		if skip[strings.ToLower(k)] {
			continue
		}
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := s.Client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, data, nil
}

func (s *Server) relay(ctx *reqCtx, w http.ResponseWriter, r *http.Request, status int, headers http.Header, data []byte) {
	ctype := strings.ToLower(headers.Get("Content-Type"))
	encoded := headers.Get("Content-Encoding") != ""
	if status == 200 && strings.Contains(ctype, "text/html") && !encoded {
		data = s.translateVendorHTML(ctx, data)
		themed := s.injectTheme(ctx, data)
		if !bytes.Equal(themed, data) {
			data = themed
			headers.Del("Content-Length")
		}
	}
	for k, vals := range headers {
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "keep-alive" || lk == "transfer-encoding" || lk == "content-length" {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if w.Header().Get("Content-Security-Policy") == "" {
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'self'; object-src 'none'")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
	}
}

func (s *Server) proxy(ctx *reqCtx, w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.ContentLength > 0 {
		body = s.readBody(r, 10_000_000)
	}
	status, headers, data, err := s.fetchUpstream(r, body, "", "")
	if err != nil {
		s.logf("upstream error: %v", err)
		s.sendHTML(w, r, 502, "<h1>502</h1><p>OnionMail HTTP is not reachable.</p>", nil)
		return
	}
	if r.URL.Path == "/api/config.api" && status == 200 {
		var js map[string]any
		if json.Unmarshal(data, &js) == nil {
			for k, v := range s.omLimits() {
				js[k] = v
			}
			if b, err := json.Marshal(js); err == nil {
				data = b
				headers.Del("Content-Length")
			}
		}
	}
	s.relay(ctx, w, r, status, headers, data)
}

func (s *Server) adminOK(r *http.Request) bool {
	if !s.adminVisible() {
		return false
	}
	status, _, data, err := s.fetchUpstream(r, nil, "/admin/ver.api", http.MethodGet)
	return err == nil && status == 200 && bytes.Contains(data, []byte(`"ok":true`))
}

func (s *Server) disabledLogon(ctx *reqCtx, w http.ResponseWriter, r *http.Request) {
	status, headers, data, err := s.fetchUpstream(r, nil, "/logon.html", http.MethodGet)
	if err != nil {
		s.sendHTML(w, r, 403, "<h1>Access denied</h1>", nil)
		return
	}
	text := ctx.t("Access denied")
	if text == "" {
		text = "Access denied"
	}
	msg := []byte(`<div class="perro" id="perrostr">` + html.EscapeString(text) + `</div>`)
	if bytes.Contains(data, []byte(`id="perrostr"`)) {
		re := regexp.MustCompile(`(?is)<div class="perro" id="perrostr">.*?</div>`)
		data = re.ReplaceAll(data, msg)
	} else if bytes.Contains(data, []byte("</body>")) {
		data = bytes.Replace(data, []byte("</body>"), append(msg, []byte("</body>")...), 1)
	}
	headers.Del("Content-Length")
	s.relay(ctx, w, r, status, headers, data)
}

func (s *Server) adminAPI(r *http.Request, path string) any {
	status, _, data, err := s.fetchUpstream(r, nil, path, http.MethodGet)
	if err != nil || status != 200 {
		return map[string]any{}
	}
	var v any
	if json.Unmarshal(data, &v) != nil {
		return map[string]any{}
	}
	if _, ok := v.(map[string]any); ok {
		return v
	}
	if _, ok := v.([]any); ok {
		return v
	}
	return map[string]any{}
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

const (
	loginWindow      = 10 * time.Minute
	loginMaxFailures = 10
	loginMapLimit    = 4096
)

func (s *Server) loginBlocked(key string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	arr := s.loginFailures[key]
	kept := arr[:0]
	for _, t := range arr {
		if now.Sub(t) < loginWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(s.loginFailures, key)
		return false
	}
	s.loginFailures[key] = kept
	return len(kept) >= loginMaxFailures
}

func (s *Server) loginFailed(key string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	arr := s.loginFailures[key]
	kept := arr[:0]
	for _, t := range arr {
		if now.Sub(t) < loginWindow {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	if len(kept) > loginMaxFailures {
		kept = kept[len(kept)-loginMaxFailures:]
	}
	s.loginFailures[key] = kept
	if len(s.loginFailures) > loginMapLimit {
		for k, v := range s.loginFailures {
			if len(v) == 0 || now.Sub(v[len(v)-1]) >= loginWindow {
				delete(s.loginFailures, k)
			}
		}
		for k := range s.loginFailures {
			if len(s.loginFailures) <= loginMapLimit {
				break
			}
			delete(s.loginFailures, k)
		}
	}
}

func (s *Server) loginSucceeded(key string) {
	s.loginMu.Lock()
	delete(s.loginFailures, key)
	s.loginMu.Unlock()
}

func (s *Server) loginFailuresFor(key string) int {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	n := 0
	for _, t := range s.loginFailures[key] {
		if now.Sub(t) < loginWindow {
			n++
		}
	}
	return n
}

func (s *Server) loginDelay(key string) time.Duration {
	n := s.loginFailuresFor(key)
	if n == 0 {
		return 0
	}
	d := 200 * time.Millisecond
	for i := 1; i < n && d < 5*time.Second; i++ {
		d *= 2
	}
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

func requestSource(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func loginKeys(user, clientID, source string) (clientKey, accountKey string) {
	accountKey = "acct:" + user
	return accountKey + "|" + clientID + "|" + source, accountKey
}

func (s *Server) clientID(w http.ResponseWriter, r *http.Request) string {
	const name = "omcid"
	if c, err := r.Cookie(name); err == nil {
		v := c.Value
		if len(v) >= 16 && len(v) <= 64 {
			ok := true
			for _, ch := range v {
				if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
					ok = false
					break
				}
			}
			if ok {
				return v
			}
		}
	}
	v := randomToken(18)
	ck := &http.Cookie{
		Name:     name,
		Value:    v,
		Path:     "/",
		MaxAge:   365 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	}
	http.SetCookie(w, ck)
	return v
}

func (s *Server) logonProxy(ctx *reqCtx, w http.ResponseWriter, r *http.Request) {
	body := s.readBody(r, 300_000)
	fields, _ := parseQuery(string(body))
	user := strings.ToLower(strings.TrimSpace(firstVal(fields, "om-lo")))
	clientKey, accountKey := loginKeys(user, s.clientID(w, r), requestSource(r))
	if user != "" && s.loginBlocked(clientKey) {
		s.logf("logon rate limited: %q", user)
		s.sendHTML(w, r, 429, s.smallPage(ctx, ctx.t("Access denied"),
			ctx.t("Too many sign-in attempts. Try again in a few minutes.")), [][2]string{{"Retry-After", "600"}})
		return
	}
	if user != "" {
		time.Sleep(s.loginDelay(accountKey))
	}
	status, headers, data, err := s.fetchUpstream(r, body, "", http.MethodPost)
	if err != nil {
		s.logf("upstream error: %v", err)
		s.sendHTML(w, r, 502, "<h1>502</h1><p>OnionMail HTTP is not reachable.</p>", nil)
		return
	}
	if user != "" {
		failed := status >= 400 || bytes.Contains(data, []byte(`name="om-lo"`)) || bytes.Contains(data, []byte(`id="perrostr"`))
		if failed {
			s.loginFailed(clientKey)
			s.loginFailed(accountKey)
		} else if status < 400 {
			s.loginSucceeded(clientKey)
			s.loginSucceeded(accountKey)
		}
	}
	s.relay(ctx, w, r, status, headers, data)
}

func (s *Server) langSelector(ctx *reqCtx) string {
	var opts strings.Builder
	keys := s.Bundle.Languages
	for _, code := range keys {
		sel := ""
		if code == ctx.lang {
			sel = " selected"
		}
		fmt.Fprintf(&opts, `<option value="%s"%s>%s</option>`, code, sel, html.EscapeString(s.Bundle.LanguageNames[code]))
	}
	return `<form class="langbox" method="get" action="/lang">` +
		`<label for="om-lang-select">` + html.EscapeString(ctx.t("Language")) + `</label>` +
		`<select id="om-lang-select" name="l" onchange="this.form.submit()">` + opts.String() + `</select>` +
		`<input type="hidden" name="next" value="` + html.EscapeString(ctx.path) + `">` +
		`</form>`
}

func (s *Server) themeToggle(ctx *reqCtx) string {
	other := "dark"
	icon := "moon.svg"
	label := ctx.t("Dark theme")
	if ctx.theme == "dark" {
		other = "light"
		icon = "sun.svg"
		label = ctx.t("Light theme")
	}
	return `<form class="themebox" method="get" action="/theme">` +
		`<input type="hidden" name="t" value="` + other + `">` +
		`<input type="hidden" name="next" value="` + html.EscapeString(ctx.path) + `">` +
		`<button type="submit" title="` + html.EscapeString(label) + `" aria-label="` + html.EscapeString(label) + `">` +
		`<img src="/theme-assets/` + icon + `" alt="" width="18" height="18"></button></form>`
}

func (s *Server) prefsSelector(ctx *reqCtx) string {
	return `<div class="om-prefs">` + s.langSelector(ctx) + s.themeToggle(ctx) + `</div>`
}

func isAdminPath(path string) bool {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return strings.HasPrefix(path, "/admin/")
}

func (s *Server) injectAdminMenu(ctx *reqCtx, data []byte) []byte {
	if !isAdminPath(ctx.path) || bytes.Contains(data, []byte(`href="/admin/server.html"`)) {
		return data
	}
	link := []byte(`<div><a href="/admin/server.html">` + html.EscapeString(ctx.t("Server")) + `</a></div>`)
	for _, pat := range []string{`(?i)<div><a href="/admin/logout\.html"`, `(?i)<div class="cli"`} {
		if loc := regexp.MustCompile(pat).FindIndex(data); loc != nil {
			out := make([]byte, 0, len(data)+len(link))
			out = append(out, data[:loc[0]]...)
			out = append(out, link...)
			out = append(out, data[loc[0]:]...)
			return out
		}
	}
	return data
}

var hexRe = regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b`)

func darkColors(text string) string {
	return hexRe.ReplaceAllStringFunc(text, func(m string) string {
		if v, ok := darkMap[strings.ToLower(m)]; ok {
			return v
		}
		return m
	})
}

func (c *reqCtx) themeCSS(base string) string {
	toggle := TOGGLE_CSS
	if c.theme == "dark" {
		return darkColors(base) + DARK_EXTRA + darkColors(TOGGLE_CSS)
	}
	return base + toggle
}

func (c *reqCtx) cloudSwapScript() string {
	suffix := "white"
	if c.theme == "dark" {
		suffix = "black"
	}
	return "<script>(function(){function s(){var r='" + suffix + "';document.querySelectorAll('img').forEach(function(i){var u=i.getAttribute('src')||'';" +
		"if(/CIEU\\.png(?:\\?|$)/.test(u)){i.src='/theme-assets/CIEU_'+r+'.png';}" +
		"else if(/CIED\\.png(?:\\?|$)/.test(u)){i.src='/theme-assets/CIED_'+r+'.png';}});}" +
		"if(document.readyState==='loading'){document.addEventListener('DOMContentLoaded',s);}else{s();}})();</script>"
}

var logoRe = regexp.MustCompile(`^/img/[A-Za-z0-9._-]+$`)

func (s *Server) currentLogo() string {
	design := omcore.Obj(s.omState(), "design")
	logo := omcore.StrDef(design, "logo", omcore.DefaultLogo)
	if !logoRe.MatchString(logo) {
		return omcore.DefaultLogo
	}
	return logo
}

func (s *Server) smallLogo() string {
	logo := omcore.Str(omcore.Obj(s.omState(), "design"), "small_logo")
	if !logoRe.MatchString(logo) {
		return ""
	}
	return logo
}

var logoSrcRe = regexp.MustCompile(`/img/(?:OM-BL\.gif|om-custom-logo\.(?:png|jpg|jpeg))`)

var extraLogoSrcRe = regexp.MustCompile(`/img/(?:ONMP\.gif|yourlogo\.gif)`)

var yourLogoLinkRe = regexp.MustCompile(`(?i)href="https?://[^"]*yourlogo\.html"`)

var logonMenuRe = regexp.MustCompile(`(?is)<div[^>]*>\s*<a[^>]*href="[^"]*logon\.html"[^>]*>.*?</a>\s*</div>`)

func hideLogonMenu(data []byte) []byte {
	return logonMenuRe.ReplaceAll(data, nil)
}

func (s *Server) smallLogoHTML() string {
	logo := s.smallLogo()
	if logo == "" {
		logo = s.currentLogo()
	}
	if logo == "" {
		return ""
	}
	return `<a class="om-toplogo" href="/" aria-label="OnionMail"><img src="` + html.EscapeString(logo) + `" alt=""></a>`
}

func (s *Server) injectLogo(data []byte) []byte {
	data = logoSrcRe.ReplaceAll(data, []byte(s.currentLogo()))
	if small := s.smallLogo(); small != "" {
		data = extraLogoSrcRe.ReplaceAll(data, []byte(small))
		data = yourLogoLinkRe.ReplaceAll(data, []byte(`href="/"`))
	}
	return data
}

func (s *Server) injectTheme(ctx *reqCtx, data []byte) []byte {
	data = s.injectLogo(data)
	if s.adminHidden() {
		data = hideLogonMenu(data)
	}
	data = s.injectAdminMenu(ctx, data)
	box := []byte(`<style id="om-prefs-style">` + ctx.themeCSS(UPSTREAM_PREFS_CSS) + `</style>` +
		`<div id="om-prefs">` + s.prefsSelector(ctx) + `</div>` + ctx.cloudSwapScript())
	lower := bytes.ToLower(data)
	if !bytes.Contains(lower, []byte("</head>")) {
		return data
	}
	if pos := bytes.Index(lower, []byte("<body")); pos != -1 {
		if j := bytes.IndexByte(data[pos:], '>'); j != -1 {
			j += pos
			data = append(append(append([]byte{}, data[:j+1]...), box...), data[j+1:]...)
			lower = bytes.ToLower(data)
		}
	} else {
		data = append(box, data...)
		lower = bytes.ToLower(data)
	}
	i := bytes.LastIndex(lower, []byte("</head>"))
	if i < 0 {
		return data
	}
	style := []byte(`<style id="om-theme">` + ctx.themeCSS(THEME) + `</style>`)
	return append(append(append([]byte{}, data[:i]...), style...), data[i:]...)
}

func (s *Server) translateVendorHTML(ctx *reqCtx, data []byte) []byte {
	table := s.Bundle.Vendor[ctx.lang]
	if len(table) == 0 {
		return data
	}
	text := string(data)
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	quoted := make([]string, len(keys))
	for i, k := range keys {
		quoted[i] = regexp.QuoteMeta(k)
	}
	re := regexp.MustCompile(`(?s)(>)(\s*)(` + strings.Join(quoted, "|") + `)(\s*)(<)`)
	text = re.ReplaceAllStringFunc(text, func(m string) string {
		sub := re.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		return sub[1] + sub[2] + html.EscapeString(table[sub[3]]) + sub[4] + sub[5]
	})
	if dyn, ok := s.Bundle.VendorDynamic[ctx.lang]; ok && len(dyn) >= 2 {
		cre := regexp.MustCompile(`(?s)(>)(\s*)Create Your (.+?) Mail Account(\s*)(<)`)
		text = cre.ReplaceAllStringFunc(text, func(m string) string {
			sub := cre.FindStringSubmatch(m)
			return sub[1] + sub[2] + html.EscapeString(strings.ReplaceAll(dyn[0], "{name}", sub[3])) + sub[4] + sub[5]
		})
		lre := regexp.MustCompile(`(?s)(>)(\s*)Logon to (.+?) Mail server(\s*)(<)`)
		text = lre.ReplaceAllStringFunc(text, func(m string) string {
			sub := lre.FindStringSubmatch(m)
			return sub[1] + sub[2] + html.EscapeString(strings.ReplaceAll(dyn[1], "{name}", sub[3])) + sub[4] + sub[5]
		})
	}
	return []byte(text)
}

func urlParseQuery(raw string) (map[string][]string, error) {
	vals, err := parseQuery(raw)
	return vals, err
}

func parseQuery(raw string) (map[string][]string, error) {
	out := map[string][]string{}
	if raw == "" {
		return out, nil
	}
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		k, err := queryUnescape(kv[0])
		if err != nil {
			return out, err
		}
		v := ""
		if len(kv) == 2 {
			v, err = queryUnescape(kv[1])
			if err != nil {
				return out, err
			}
		}
		out[k] = append(out[k], v)
	}
	return out, nil
}

func queryUnescape(s string) (string, error) {
	s = strings.ReplaceAll(s, "+", " ")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			n, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if err == nil {
				b.WriteByte(byte(n))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String(), nil
}

func formEncode(pairs [][2]string) string {
	var b strings.Builder
	for i, kv := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(queryEscape(kv[0]))
		b.WriteByte('=')
		b.WriteString(queryEscape(kv[1]))
	}
	return b.String()
}

func queryEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' {
			b.WriteByte('+')
		} else if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func parseURIPath(raw string) string {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i]
	}
	return raw
}
