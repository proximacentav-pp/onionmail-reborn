package omcore

import (
	"os"
	"path/filepath"
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
