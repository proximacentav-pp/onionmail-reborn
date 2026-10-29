package omcore

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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

// OpenControlSSHTunnel starts a local SSH tunnel to the encrypted ControlPort
// using the internal TUI key and returns a connection to the plain ControlPort.
func OpenControlSSHTunnel(sshPort, ctrlPort int, user string) (net.Conn, func(), error) {
	_, clientKey, knownHosts, _, _, _ := ControlSSHPaths()
	if _, err := os.Stat(clientKey); err != nil {
		return nil, nil, fmt.Errorf("SSH keys are not ready; restart the container after enabling")
	}
	if _, err := os.Stat(knownHosts); err != nil {
		return nil, nil, fmt.Errorf("SSH host key is not known; restart the container after enabling")
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		localPort, err := freeLoopbackPort()
		if err != nil {
			return nil, nil, err
		}
		c, cleanup, err := openControlSSHTunnelOnce(sshPort, ctrlPort, user, localPort, clientKey, knownHosts)
		if err == nil {
			return c, cleanup, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

func openControlSSHTunnelOnce(sshPort, ctrlPort int, user string, localPort int, clientKey, knownHosts string) (net.Conn, func(), error) {
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
