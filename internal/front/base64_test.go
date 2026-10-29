package front

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestWriteBase64PartAlwaysTerminatesLine(t *testing.T) {
	for _, n := range []int{0, 1, 55, 56, 57, 75, 76, 77, 112, 114, 133, 1024} {
		body := strings.Repeat("a", n)
		var buf bytes.Buffer
		if err := writeBase64Part(&buf, strings.NewReader(body)); err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		out := buf.String()
		if n > 0 && !strings.HasSuffix(out, "\r\n") {
			t.Fatalf("n=%d: output does not end with CRLF: %q", n, out)
		}
		for _, line := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
			if len(line) > 76 {
				t.Fatalf("n=%d: line longer than 76 chars: %d", n, len(line))
			}
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out, "\r\n", ""))
		if err != nil {
			t.Fatalf("n=%d: decode: %v", n, err)
		}
		if string(decoded) != body {
			t.Fatalf("n=%d: roundtrip mismatch", n)
		}
	}
}
