// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// pasvServer is an FTP server whose PASV reply names 127.0.0.2 while the
// control connection is to 127.0.0.1. Both addresses accept the data
// connection on the same port, and the server records which one was used.
type pasvServer struct {
	ln net.Listener

	mu  sync.Mutex
	via string
}

func startPASVServer(t *testing.T) *pasvServer {
	t.Helper()
	if probe, err := net.Listen("tcp", "127.0.0.2:0"); err != nil {
		t.Skipf("cannot listen on 127.0.0.2: %s", err)
	} else {
		probe.Close()
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &pasvServer{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *pasvServer) usedHost() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.via
}

func (s *pasvServer) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	reply := func(format string, args ...interface{}) { fmt.Fprintf(c, format+"\r\n", args...) }
	reply("220 ready")

	var data []net.Listener
	defer func() {
		for _, l := range data {
			l.Close()
		}
	}()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		verb, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch strings.ToUpper(verb) {
		case "USER":
			reply("331 password please")
		case "PASS":
			reply("230 logged in")
		case "TYPE", "OPTS":
			reply("200 ok")
		case "PWD":
			reply(`257 "/" is the current directory`)
		case "PASV":
			control, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				reply("425 no data connection")
				continue
			}
			port := control.Addr().(*net.TCPAddr).Port
			announced, err := net.Listen("tcp", fmt.Sprintf("127.0.0.2:%d", port))
			if err != nil {
				control.Close()
				reply("425 no data connection")
				continue
			}
			data = []net.Listener{control, announced}
			reply("227 Entering Passive Mode (127,0,0,2,%d,%d)", port>>8, port&0xff)
		case "LIST":
			if len(data) == 0 {
				reply("425 use PASV first")
				continue
			}
			reply("150 opening data connection")
			conns := make(chan [2]interface{}, 2)
			for _, l := range data {
				go func(l net.Listener) {
					dc, err := l.Accept()
					if err == nil {
						conns <- [2]interface{}{dc, l.Addr().(*net.TCPAddr).IP.String()}
					}
				}(l)
			}
			select {
			case got := <-conns:
				dc := got[0].(net.Conn)
				s.mu.Lock()
				s.via = got[1].(string)
				s.mu.Unlock()
				fmt.Fprint(dc, "-rw-r--r--   1 u g 5 Jan 01  2026 file.txt\r\n")
				dc.Close()
				reply("226 transfer complete")
			case <-time.After(2 * time.Second):
				reply("425 nobody connected")
			}
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("500 command not understood")
		}
	}
}

func listWithPASV(t *testing.T, s *pasvServer, ignore bool) {
	t.Helper()
	cfg := Config{
		User:           "u",
		Password:       "p",
		DisableEPSV:    true,
		IgnorePASVHost: ignore,
		Timeout:        3 * time.Second,
	}
	c, err := DialConfig(cfg, s.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	entries, err := c.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir: %s", err)
	}
	if len(entries) != 1 || entries[0].Name() != "file.txt" {
		t.Errorf("listed %v, want file.txt", entries)
	}
}

func TestPASVHostIsIgnoredWhenAsked(t *testing.T) {
	s := startPASVServer(t)

	listWithPASV(t, s, true)

	if got := s.usedHost(); got != "127.0.0.1" {
		t.Errorf("data connection went to %s, want the control connection's host 127.0.0.1", got)
	}
}

func TestPASVHostIsUsedByDefault(t *testing.T) {
	s := startPASVServer(t)

	listWithPASV(t, s, false)

	if got := s.usedHost(); got != "127.0.0.2" {
		t.Errorf("data connection went to %s, want the host the server announced, 127.0.0.2", got)
	}
}
