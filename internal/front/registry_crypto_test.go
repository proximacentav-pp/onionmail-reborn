package front

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServerWithKey(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "registry.key")
	if err := os.WriteFile(keyPath, []byte(strings.Repeat("k", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Server{Registry: filepath.Join(dir, "registry.json"), RegistryKey: keyPath, Log: os.Stderr}
}

func TestRegistrySecretRoundtrip(t *testing.T) {
	s := testServerWithKey(t)
	enc, err := s.encryptSecret("p@ss\r\nword")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, "p@ss") {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := s.decryptSecret(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "p@ss\r\nword" {
		t.Fatalf("roundtrip = %q", got)
	}
}

func TestRegistryMigrationRemovesPlaintext(t *testing.T) {
	s := testServerWithKey(t)
	data := registryData{Users: map[string]map[string]any{
		"alice": {"smtp": "smtp-pass", "pop3": "pop3-pass", "created": 1},
	}}
	if err := s.saveRegistry(data); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateRegistrySecrets(); err != nil {
		t.Fatal(err)
	}
	entry := s.loadRegistry().Users["alice"]
	if _, ok := entry["smtp"]; ok {
		t.Fatalf("plaintext smtp left: %#v", entry)
	}
	if _, ok := entry["pop3"]; ok {
		t.Fatalf("plaintext pop3 left: %#v", entry)
	}
	if got := s.entryCredential(entry, "smtp"); got != "smtp-pass" {
		t.Fatalf("smtp = %q", got)
	}
	if got := s.entryCredential(entry, "pop3"); got != "pop3-pass" {
		t.Fatalf("pop3 = %q", got)
	}
}

func TestRegistryWithoutKeyDoesNotStorePlaintext(t *testing.T) {
	dir := t.TempDir()
	s := &Server{Registry: filepath.Join(dir, "registry.json"), RegistryKey: filepath.Join(dir, "missing.key"), Log: os.Stderr}
	s.registerUser("bob", "smtp-pass", "pop3-pass", "", 0)
	entry := s.loadRegistry().Users["bob"]
	if entry == nil {
		return
	}
	if entry["smtp"] != nil || entry["pop3"] != nil {
		t.Fatalf("plaintext credentials stored without key: %#v", entry)
	}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		origin, referer, host string
		want                  bool
	}{
		{"", "", "abc.onion", true},
		{"http://abc.onion", "", "abc.onion", true},
		{"http://evil.example", "", "abc.onion", false},
		{"", "http://abc.onion/admin/index.html", "abc.onion", true},
		{"", "http://evil.example/x", "abc.onion", false},
		{"null", "", "abc.onion", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "http://abc.onion/admin/index.html", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.referer != "" {
			r.Header.Set("Referer", c.referer)
		}
		if got := sameOrigin(r); got != c.want {
			t.Fatalf("origin=%q referer=%q: got %v want %v", c.origin, c.referer, got, c.want)
		}
	}
}
