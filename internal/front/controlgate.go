package front

import (
	"io"
	"net"
	"strconv"
	"time"

	"onionmail/internal/omcore"
)

func (s *Server) startControlGate() {
	enabled, port := omcore.ControlFromState(omcore.LoadState())
	if !enabled {
		s.logf("control gatekeeper disabled")
		return
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(omcore.ControlLocalIP, strconv.Itoa(port)))
	if err != nil {
		s.logf("control gatekeeper listen %d: %v", port, err)
		return
	}
	s.logf("control gatekeeper on %s:%d -> %s:%d", omcore.ControlLocalIP, port, omcore.ControlUpstreamIP, port)
	for {
		c, err := ln.Accept()
		if err != nil {
			s.logf("control gatekeeper stopped: %v", err)
			return
		}
		go s.proxyControl(c, port)
	}
}

func (s *Server) proxyControl(c net.Conn, port int) {
	defer c.Close()
	up, err := net.DialTimeout("tcp", net.JoinHostPort(omcore.ControlUpstreamIP, strconv.Itoa(port)), 10*time.Second)
	if err != nil {
		s.logf("control gatekeeper upstream %d: %v", port, err)
		return
	}
	defer up.Close()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(up, c)
		if tc, ok := up.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(c, up)
		if tc, ok := c.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
}
