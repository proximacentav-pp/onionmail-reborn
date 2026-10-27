package front

import (
	"path/filepath"
	"testing"
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
