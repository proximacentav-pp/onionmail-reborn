package main

import (
	"testing"

	"onionmail/internal/omcore"
)

func TestSetSkipTorRoundTrip(t *testing.T) {
	st := omcore.State{"tor": map[string]any{"mode": "external", "port": 9050, "internal_port": 19050}}
	changed, _ := setSkipTor(st, true)
	if !changed {
		t.Fatal("enable did not change state")
	}
	if got := omcore.Str(omcore.Obj(st, "tor"), "mode"); got != "off" {
		t.Fatalf("mode = %q", got)
	}
	if changed, _ := setSkipTor(st, true); changed {
		t.Fatal("second enable changed state")
	}
	changed, _ = setSkipTor(st, false)
	if !changed {
		t.Fatal("disable did not change state")
	}
	tor := omcore.Obj(st, "tor")
	if got := omcore.Str(tor, "mode"); got != "external" {
		t.Fatalf("restored mode = %q", got)
	}
	if _, ok := tor[skipTorPrevKey]; ok {
		t.Fatal("previous mode was not removed")
	}
}

func TestSetSkipTorRestoresInternal(t *testing.T) {
	st := omcore.State{"tor": map[string]any{"mode": "internal", "port": 9050, "internal_port": 19050}}
	setSkipTor(st, true)
	setSkipTor(st, false)
	if got := omcore.Str(omcore.Obj(st, "tor"), "mode"); got != "internal" {
		t.Fatalf("restored mode = %q", got)
	}
}
