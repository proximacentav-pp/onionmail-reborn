package front

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"onionmail/internal/olang"
)

func TestWebmailBlockedRegistry(t *testing.T) {
	s := &Server{Registry: filepath.Join(t.TempDir(), "registry.json")}
	user := "webtest-block"

	if s.webmailBlocked(user) {
		t.Fatal("new user is blocked")
	}
	s.setWebmailBlocked(user, true)
	if !s.webmailBlocked(user) {
		t.Fatal("user is not blocked after set")
	}
	if !s.webmailBlocked(" WebTest-Block ") {
		t.Fatal("lookup is not case/space insensitive")
	}
	if got := s.loadRegistry().Users[user]["webmail_blocked"]; got != true {
		t.Fatalf("registry flag = %v", got)
	}
	s.setWebmailBlocked(user, false)
	if s.webmailBlocked(user) {
		t.Fatal("user is still blocked after unset")
	}
	if _, ok := s.loadRegistry().Users[user]["webmail_blocked"]; ok {
		t.Fatal("registry flag was not removed")
	}
}

func TestRegisterPostStoresWebmailBlock(t *testing.T) {
	var posted string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		posted = string(b)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "Subscription complete\nSMTP Password: smtppass123\nPOP3 Password: pop3pass456\n")
	}))
	defer up.Close()

	dir := t.TempDir()
	b, err := olang.Load(filepath.Join("..", "..", "languages.json"))
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "registry.key")
	if err := os.WriteFile(keyPath, []byte(strings.Repeat("k", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Upstream:    strings.TrimPrefix(up.URL, "http://"),
		Registry:    filepath.Join(dir, "registry.json"),
		RegistryKey: keyPath,
		Client:      up.Client(),
		Bundle:      b,
		Log:         io.Discard,
	}
	ctx := &reqCtx{s: s, lang: "en", theme: "light", path: "/register.html"}
	r := httptest.NewRequest(http.MethodPost, "/register.html", strings.NewReader("om-localpart=webtestblock&om-noweb=1&om-pass=secret&om-pass2=secret"))
	w := httptest.NewRecorder()
	s.registerPost(ctx, w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if strings.Contains(posted, "om-noweb") {
		t.Fatal("om-noweb was forwarded upstream")
	}
	if !s.webmailBlocked("webtestblock") {
		t.Fatal("registration did not store the webmail block")
	}
	entry := s.loadRegistry().Users["webtestblock"]
	if entry["smtp"] != nil || entry["pop3"] != nil {
		t.Fatalf("plaintext credentials stored: %#v", entry)
	}
	if got := s.entryCredential(entry, "smtp"); got != "smtppass123" {
		t.Fatalf("smtp credential = %q", got)
	}
	if got := s.entryCredential(entry, "pop3"); got != "pop3pass456" {
		t.Fatalf("pop3 credential = %q", got)
	}
}
