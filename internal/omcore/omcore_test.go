package omcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteKeepDefaultModeIsPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")
	if err := WriteKeep(path, "secret-data", nil); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o007 != 0 {
		t.Fatalf("default mode exposes secrets: %04o", st.Mode().Perm())
	}
	if b, _ := os.ReadFile(path); string(b) != "secret-data" {
		t.Fatalf("content = %q", b)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".*omtmp-*"))
	if len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

func TestWriteKeepPreservesExplicitMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "torrc")
	mode := os.FileMode(0o644)
	if err := WriteKeep(path, "x", &mode); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %04o", st.Mode().Perm())
	}
}

func TestSetNickPathsOnlyKnownValues(t *testing.T) {
	in := strings.Join([]string{
		"MailDir\t/var/onionmail/om",
		"SMTPServer\tom.onion",
		"PassWD\tomsecret",
		"Onion\tomqy2xyzabcdefgh.onion",
		"# comment about om",
		"DisplayName\tom server",
		"",
	}, "\n")
	out := SetNickPaths(in, "om", "alpha")
	for _, want := range []string{
		"MailDir\t/var/onionmail/alpha",
		"SMTPServer\talpha.onion",
		"PassWD\tomsecret",
		"Onion\tomqy2xyzabcdefgh.onion",
		"# comment about om",
		"DisplayName\tom server",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestNormalizeStateSeparatesSSHPassword(t *testing.T) {
	st := NormalizeState(map[string]any{
		"control": map[string]any{
			"root_password": "root-secret-1",
			"ssh_enabled":   true,
		},
	}, DeriveState())
	ctrl := Obj(st, "control")
	root := Str(ctrl, "root_password")
	ssh := Str(ctrl, "ssh_password")
	if root == "" || ssh == "" {
		t.Fatalf("passwords must be set: root=%q ssh=%q", root, ssh)
	}
	if root == ssh {
		t.Fatal("SSH password must differ from control root password")
	}
	if got := ControlSSHPass(st); got != ssh {
		t.Fatalf("ControlSSHPass=%q want %q", got, ssh)
	}
	again := NormalizeState(map[string]any{
		"control": map[string]any{
			"root_password": "root-secret-1",
			"ssh_enabled":   true,
			"ssh_password":  ssh,
		},
	}, DeriveState())
	if got := Str(Obj(again, "control"), "ssh_password"); got != ssh {
		t.Fatalf("existing SSH password was replaced: %q", got)
	}
}

func TestControlSSHPassFallback(t *testing.T) {
	st := map[string]any{"control": map[string]any{"root_password": "root-secret-1"}}
	if got := ControlSSHPass(st); got != "root-secret-1" {
		t.Fatalf("fallback=%q", got)
	}
}
