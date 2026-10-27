package main

import (
	"fmt"
	"os"
	"strings"

	"onionmail/internal/omcore"
)

const skipTorPrevKey = "skip_tor_prev"

func setSkipTor(st omcore.State, enable bool) (bool, string) {
	tor, ok := st["tor"].(map[string]any)
	if !ok {
		tor = map[string]any{}
		st["tor"] = tor
	}
	mode := omcore.StrDef(tor, "mode", "external")
	if enable {
		if mode == "off" {
			return false, "skip-tor is already enabled"
		}
		tor[skipTorPrevKey] = mode
		tor["mode"] = "off"
		return true, "skip-tor enabled: onion disabled, localhost only, boot waits skipped"
	}
	if mode != "off" {
		return false, "skip-tor is not enabled"
	}
	prev := omcore.Str(tor, skipTorPrevKey)
	if prev != "external" && prev != "internal" {
		prev = "external"
	}
	tor["mode"] = prev
	delete(tor, skipTorPrevKey)
	return true, "Tor mode restored to " + prev
}

func skipTorCmd(args []string) int {
	usage := func() {
		fmt.Fprintln(os.Stderr, "Usage: om skip-tor [on|off|status]")
	}
	st := omcore.LoadState()
	action := "on"
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch action {
	case "status", "show":
		mode := omcore.StrDef(omcore.Obj(st, "tor"), "mode", "external")
		if mode == "off" {
			fmt.Println("skip-tor: on (onion disabled, localhost only)")
		} else {
			fmt.Println("skip-tor: off (Tor mode: " + mode + ")")
		}
		return 0
	case "on", "enable":
		action = "on"
	case "off", "disable":
		action = "off"
	default:
		usage()
		return 1
	}
	changed, msg := setSkipTor(st, action == "on")
	fmt.Println(msg)
	if !changed {
		return 0
	}
	ok, errs := omcore.Save(st, "tor")
	if !ok {
		fmt.Fprintln(os.Stderr, "om skip-tor: "+strings.Join(errs, "; "))
		return 1
	}
	fmt.Println("Restarting OnionMail to apply the changes...")
	omcore.Restart()
	return 0
}
