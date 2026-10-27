package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"onionmail/internal/olang"
	"onionmail/internal/omcore"
	"onionmail/internal/omtui"
)

func controlportConnect(st omcore.State, encrypted bool) int {
	enabled, port := omcore.ControlFromState(st)
	if !enabled {
		fmt.Fprintln(os.Stderr, "om controlport: ControlPort is disabled")
		return 1
	}
	var conn net.Conn
	var cleanup func()
	var label string
	var err error
	if encrypted {
		sshEnabled, sshPort, sshUser, _ := omcore.ControlSSHFromState(st)
		if !sshEnabled {
			fmt.Fprintln(os.Stderr, "om controlport: encrypted ControlPort is disabled")
			return 1
		}
		conn, cleanup, err = omcore.OpenControlSSHTunnel(sshPort, port, sshUser)
		label = fmt.Sprintf("SSH %s@127.0.0.1:%d -> 127.0.0.1:%d", sshUser, sshPort, port)
	} else {
		conn, err = net.DialTimeout("tcp", net.JoinHostPort(omcore.ControlLocalIP, strconv.Itoa(port)), 5*time.Second)
		label = fmt.Sprintf("plain 127.0.0.1:%d", port)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "om controlport: "+err.Error())
		return 1
	}
	defer conn.Close()
	if cleanup != nil {
		defer cleanup()
	}
	b, err := olang.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "om controlport: "+err.Error())
		return 1
	}
	return omtui.RunControlTelnet(st, b, conn, label)
}
