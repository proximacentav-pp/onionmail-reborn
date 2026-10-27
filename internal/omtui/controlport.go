package omtui

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"onionmail/internal/omcore"
)

const controlMaxBuffer = 200_000

func (u *UI) paintPlain(row, col int, text string) {
	if row < 1 || row > u.rows || col < 1 || col > u.cols {
		return
	}
	fmt.Printf("\x1b[%d;%dH%s", row, col, trunc(text, u.cols-col+1))
}

func fdSet(fd int, set *syscall.FdSet) {
	set.Bits[fd/64] |= int64(1) << (uint(fd) % 64)
}

func fdIsSet(fd int, set *syscall.FdSet) bool {
	return set.Bits[fd/64]&(int64(1)<<(uint(fd)%64)) != 0
}

func sanitizeControlOutput(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", "    ")
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wrapControlLines(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, raw := range strings.Split(s, "\n") {
		r := []rune(raw)
		if len(r) == 0 {
			out = append(out, "")
			continue
		}
		for len(r) > width {
			out = append(out, string(r[:width]))
			r = r[width:]
		}
		out = append(out, string(r))
	}
	return out
}

func controlHelpLines() []string {
	return []string{
		"server <nick> <pass>      login as server",
		"user <localpart> <pass>   login as user",
		"su <rootpass>             login as root",
		"sux <md5>                 login as root (hash)",
		"ver / vers                version",
		"stat / status             statistics",
		"access                    access information",
		"getkey                    RSA public key",
		"bestexit                  best exit server",
		"firends / frienderr       friends lists",
		"sslcert                   SSL certificate",
		"spam create / list        spam list",
		"spam add <address>",
		"spam del <entry>",
		"spam check <address>",
		"dnsbl <ipaddress>",
		"iplist                    IP black list",
		"vmatreg <address>",
		"addusr <localpart>",
		"ovrusr <localpart>",
		"deluser <localpart>",
		"vrfy <localpart>",
		"addpgpusr <localpart>",
		"vouchermk <minutes>",
		"voucherchk <voucher>",
		"voucher <voucher>",
		"list <addr.list> <user> [pass]",
		"mklist <addr.list> <owner> <title>",
		"addalias <alias> <localpart>",
		"delalias <alias>",
		"alias <alias>",
		"ssleid [clear|del|chg|no] ...",
		"sslclear <host>",
		"sslcheck <host> [Y|N]",
		"torid / toridf            Tor identity",
		"captcha                   test captcha",
		"rmmanifest <address>",
		"fsave <name> / fload / fdel",
		"send <address>",
		"rsetcnt                   reset limits",
		"exit                      leave mode",
		"quit                      close session",
		"",
		"SSH tunnel:",
		"  ssh -p <enc-port> <user>@127.0.0.1 -N \\",
		"      -L <ctrl-port>:127.0.0.1:<ctrl-port>",
		"  SSH password = root password",
		"  (encrypted port listens on 127.0.0.1 only)",
	}
}

func (u *UI) controlPortScreen() {
	ctrl := omcore.Obj(u.st, "control")
	enabled, port := omcore.ControlFromState(u.st)
	if !enabled {
		u.setMsg(u.t("ControlPort is disabled. Enable it and restart."), false)
		return
	}
	rawMode := omcore.StrDef(ctrl, "ssh_mode", "unactive")
	sshEnabled, sshPort, sshUser, _ := omcore.ControlSSHFromState(u.st)
	var conn net.Conn
	var cleanup func()
	var label string
	if rawMode == "sshtunnel" {
		if !sshEnabled {
			u.setMsg(u.t("Encrypted ControlPort is disabled. Enable it and restart."), false)
			return
		}
		var err error
		conn, cleanup, err = openControlSSHTunnel(sshPort, port, sshUser)
		if err != nil {
			u.setMsg(u.t("SSH tunnel error")+": "+err.Error(), false)
			return
		}
		label = fmt.Sprintf("SSH %s@127.0.0.1:%d -> 127.0.0.1:%d", sshUser, sshPort, port)
	} else {
		var err error
		addr := net.JoinHostPort(omcore.ControlLocalIP, strconv.Itoa(port))
		conn, err = net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			u.setMsg(u.t("ControlPort connection error")+": "+err.Error(), false)
			return
		}
		label = fmt.Sprintf("plain 127.0.0.1:%d", port)
	}
	defer conn.Close()
	if cleanup != nil {
		defer cleanup()
	}
	u.controlTelnet(conn, label)
	u.setMsg("", true)
}

func freeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("cannot allocate a local port")
	}
	return addr.Port, nil
}

func openControlSSHTunnel(sshPort, ctrlPort int, user string) (net.Conn, func(), error) {
	_, clientKey, knownHosts, _, _, _ := omcore.ControlSSHPaths()
	if _, err := os.Stat(clientKey); err != nil {
		return nil, nil, fmt.Errorf("SSH keys are not ready; restart the container after enabling")
	}
	if _, err := os.Stat(knownHosts); err != nil {
		return nil, nil, fmt.Errorf("SSH host key is not known; restart the container after enabling")
	}
	localPort, err := freeLoopbackPort()
	if err != nil {
		return nil, nil, err
	}
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return nil, nil, fmt.Errorf("ssh client not found")
	}
	args := []string{
		"-i", clientKey,
		"-p", strconv.Itoa(sshPort),
		"-N",
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + knownHosts,
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=15",
		"-o", "LogLevel=ERROR",
		"-L", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", localPort, ctrlPort),
		user + "@127.0.0.1",
	}
	cmd := exec.Command(sshPath, args...)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	deadline := time.Now().Add(8 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, 400*time.Millisecond)
		if err == nil {
			cleanup := func() {
				_ = cmd.Process.Kill()
				<-waitCh
			}
			return c, cleanup, nil
		}
		select {
		case werr := <-waitCh:
			return nil, nil, fmt.Errorf("%v: %s", werr, strings.TrimSpace(errBuf.String()))
		default:
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			<-waitCh
			return nil, nil, fmt.Errorf("timeout: %s", strings.TrimSpace(errBuf.String()))
		}
		time.Sleep(120 * time.Millisecond)
	}
}

func (u *UI) controlTelnet(conn net.Conn, label string) {
	defer fmt.Print("\x1b[?25l")
	fmt.Print("\x1b[?25h")
	var buf strings.Builder
	var line []rune
	cur := 0
	help := false
	helpOff := 0
	outOff := 0
	closed := false

	appendOut := func(s string) {
		s = sanitizeControlOutput(s)
		if s == "" {
			return
		}
		buf.WriteString(s)
		if buf.Len() > controlMaxBuffer {
			v := buf.String()
			v = v[len(v)-controlMaxBuffer:]
			if i := strings.IndexByte(v, '\n'); i >= 0 {
				v = v[i+1:]
			}
			buf.Reset()
			buf.WriteString(v)
		}
	}
	send := func(s string) {
		if closed {
			return
		}
		_, _ = conn.Write([]byte(s))
	}
	readConn := func() bool {
		if closed {
			return false
		}
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		tmp := make([]byte, 8192)
		n, err := conn.Read(tmp)
		got := n > 0
		if got {
			appendOut(string(tmp[:n]))
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return got
			}
			closed = true
			appendOut("\n[connection closed]\n")
			got = true
		}
		return got
	}
	handleKey := func(k string) bool {
		switch k {
		case "ctrl-]":
			return false
		case "ctrl-a":
			help = !help
			helpOff = 0
		case "enter":
			cmd := string(line)
			appendOut("> " + cmd + "\n")
			send(cmd + "\n")
			line = nil
			cur = 0
			outOff = 0
		case "backspace":
			if cur > 0 {
				line = append(line[:cur-1], line[cur:]...)
				cur--
			}
		case "delete":
			if cur < len(line) {
				line = append(line[:cur], line[cur+1:]...)
			}
		case "left":
			if cur > 0 {
				cur--
			}
		case "right":
			if cur < len(line) {
				cur++
			}
		case "home":
			cur = 0
		case "end":
			cur = len(line)
		case "up":
			if help {
				helpOff = max(0, helpOff-1)
			} else {
				outOff++
			}
		case "down":
			if help {
				helpOff++
			} else {
				outOff = max(0, outOff-1)
			}
		case "pgup":
			if help {
				helpOff = max(0, helpOff-max(1, u.rows-5))
			} else {
				outOff += max(1, u.rows-4)
			}
		case "pgdn":
			if help {
				helpOff += max(1, u.rows-5)
			} else {
				outOff = max(0, outOff-max(1, u.rows-4))
			}
		default:
			if strings.HasPrefix(k, "text:") {
				for _, r := range []rune(k[5:]) {
					line = append(line[:cur], append([]rune{r}, line[cur:]...)...)
					cur++
				}
			}
		}
		return true
	}

	if label != "" {
		appendOut("[" + label + "]\n")
	}
	dirty := true
	lastRows, lastCols := u.rows, u.cols
	for {
		u.termSize()
		if u.rows != lastRows || u.cols != lastCols {
			lastRows, lastCols = u.rows, u.cols
			dirty = true
		}
		if !dirty {
			stdinFd := int(os.Stdin.Fd())
			connFd := int(connFdOf(conn))
			maxFd := stdinFd
			if !closed && connFd > maxFd {
				maxFd = connFd
			}
			fds := &syscall.FdSet{}
			fdSet(stdinFd, fds)
			if !closed && connFd > 0 {
				fdSet(connFd, fds)
			}
			tv := syscall.Timeval{Usec: 100_000}
			n, err := syscall.Select(maxFd+1, fds, nil, nil, &tv)
			if err != nil || n <= 0 {
				continue
			}
			if !closed && connFd > 0 && fdIsSet(connFd, fds) {
				if readConn() {
					dirty = true
				}
			}
			if fdIsSet(stdinFd, fds) {
				k := u.readKey()
				if !handleKey(k) {
					return
				}
				dirty = true
			}
			continue
		}
		helpW := 0
		if help {
			helpW = min(46, max(24, u.cols/2))
			if helpW > u.cols-20 {
				helpW = u.cols / 2
			}
		}
		telW := u.cols
		if help {
			telW = u.cols - helpW - 1
		}
		if telW < 10 {
			telW = 10
		}
		outH := max(1, u.rows-2)
		lines := wrapControlLines(buf.String(), telW)
		maxOff := max(0, len(lines)-outH)
		if outOff > maxOff {
			outOff = maxOff
		}
		start := max(0, len(lines)-outH-outOff)

		fmt.Print("\x1b[0m\x1b[H\x1b[2J")
		for i := 0; i < outH && start+i < len(lines); i++ {
			u.paintPlain(1+i, 1, lines[start+i])
		}

		inputRow := u.rows - 1
		prompt := "> "
		shown := []rune(prompt + string(line))
		pos := len([]rune(prompt)) + cur
		maxw := max(1, telW)
		off := 0
		if pos >= maxw {
			off = pos - maxw + 1
		}
		if off > len(shown) {
			off = len(shown)
		}
		u.paintPlain(inputRow, 1, string(shown[off:]))
		fmt.Print("\x1b[K")

		if help {
			sep := telW + 1
			for r := 1; r <= outH+1; r++ {
				u.paintPlain(r, sep, "|")
			}
			hx := sep + 2
			hw := max(1, u.cols-hx+1)
			helpLines := controlHelpLines()
			header := u.t("ControlPort help")
			u.paintPlain(1, hx, header)
			body := wrapControlLines(strings.Join(helpLines, "\n"), hw)
			hOff := helpOff
			if hOff > max(0, len(body)-(outH-1)) {
				hOff = max(0, len(body)-(outH-1))
			}
			for i := 0; i < outH-1 && hOff+i < len(body); i++ {
				u.paintPlain(2+i, hx, body[hOff+i])
			}
		}

		footer := "Ctrl+] exit   Ctrl+A help   Enter send"
		if help {
			footer += "   PgUp/PgDn scroll help"
		}
		if outOff > 0 {
			footer += fmt.Sprintf("   [scroll %d]", outOff)
		}
		u.paintPlain(u.rows, 1, footer)
		col := 1 + pos - off
		if col < 1 {
			col = 1
		}
		fmt.Printf("\x1b[%d;%dH", inputRow, col)
		dirty = false
	}
}

func connFdOf(conn net.Conn) uintptr {
	type syscallConner interface {
		SyscallConn() (syscall.RawConn, error)
	}
	if c, ok := conn.(syscallConner); ok {
		var fd uintptr
		raw, err := c.SyscallConn()
		if err == nil {
			_ = raw.Control(func(f uintptr) { fd = f })
			return fd
		}
	}
	return 0
}
