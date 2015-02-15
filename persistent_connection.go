// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"fmt"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// Represents a single connection to an FTP server.
type persistentConn struct {
	// control socket
	controlConn net.Conn

	// control socket read/write helpers
	reader *textproto.Reader
	writer *textproto.Writer

	config Config
	t0     time.Time

	// has this connection encountered an unrecoverable error
	broken bool

	// index of this connection (used for logging context and
	// round-roubin host selection)
	idx int

	// map of ftp features available on server
	features map[string]string
}

func (pconn *persistentConn) close() {
	pconn.debug("closed")
	if pconn.controlConn != nil {
		pconn.controlConn.Close()
	}
}

func (pconn *persistentConn) sendCommand(f string, args ...interface{}) (int, string, error) {
	err := pconn.writer.PrintfLine(f, args...)
	if err != nil {
		return 0, "", fmt.Errorf("error writing command: %s", err)
	}

	code, msg, err := pconn.reader.ReadResponse(0)
	if err != nil {
		return 0, "", fmt.Errorf("error reading response: %s")
	}

	return code, msg, err
}

func (pconn *persistentConn) debug(f string, args ...interface{}) {
	if pconn.config.Logger == nil {
		return
	}

	msg := fmt.Sprintf("goftp: #%d %.3f %s\n",
		pconn.idx,
		time.Now().Sub(pconn.t0).Seconds(),
		fmt.Sprintf(f, args...),
	)

	pconn.config.Logger.Write([]byte(msg))
}

func (pconn *persistentConn) fetchFeatures() error {
	pconn.debug("fetching features")

	code, msg, err := pconn.sendCommand("FEAT")
	if err != nil {
		return fmt.Errorf("error fetching features: %s", err)
	}

	if !positiveCompletionReply(code) {
		pconn.debug("server doesn't support FEAT: %d (%s)", code, msg)
		return nil
	}

	for _, line := range strings.Split(msg, "\n") {
		if line[0] == ' ' {
			parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
			if len(parts) == 1 {
				pconn.features[parts[0]] = ""
			} else if len(parts) == 2 {
				pconn.features[parts[0]] = parts[1]
			}
		}
	}

	return nil
}

func (pconn *persistentConn) logInConn() error {
	if pconn.config.User == "" {
		return nil
	}

	pconn.debug("logging in as user %s", pconn.config.User)
	code, msg, err := pconn.sendCommand("USER %s", pconn.config.User)
	if err != nil {
		return err
	}

	if code == ReplyNeedPassword {
		pconn.debug("sending password")
		code, msg, err = pconn.sendCommand("PASS %s", pconn.config.Password)
		if err != nil {
			return err
		}
	}

	if !positiveCompletionReply(code) {
		return fmt.Errorf("unexpected response: %d (%s)", code, msg)
	}

	return nil
}

func (pconn *persistentConn) openDataConn() (net.Conn, error) {
	pconn.debug("requesting PASV mode")
	code, msg, err := pconn.sendCommand("PASV")
	if err != nil {
		return nil, err
	}

	if code != ReplyEnteringPassiveMode {
		return nil, fmt.Errorf("server doesn't support passive mode (%d %s)", code, msg)
	}

	parseError := fmt.Errorf("error parsing PASV response (%s)", msg)

	// "Entering Passive Mode (162,138,208,11,223,57)."
	openParen := strings.Index(msg, "(")
	if openParen == -1 {
		return nil, parseError
	}

	closeParen := strings.LastIndex(msg, ")")
	if closeParen == -1 || closeParen < openParen {
		return nil, parseError
	}

	addrParts := strings.Split(msg[openParen+1:closeParen], ",")
	if len(addrParts) != 6 {
		return nil, parseError
	}

	ip := net.ParseIP(strings.Join(addrParts[0:4], "."))
	if ip == nil {
		return nil, parseError
	}

	port := 0
	for i, part := range addrParts[4:6] {
		portOctet, err := strconv.Atoi(part)
		if err != nil {
			return nil, parseError
		}
		port |= portOctet << (byte(1-i) * 8)
	}

	pconn.debug("opening data connection to %s:%d", ip.String(), port)

	return net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip.String(), port), pconn.config.Timeout)
}

func (pconn *persistentConn) setType(t string) error {
	pconn.debug("switching to TYPE %s", t)
	code, msg, err := pconn.sendCommand("TYPE %s", t)
	if err != nil {
		return err
	}

	if code != ReplyCommandOkay {
		return fmt.Errorf("unexpected response: %d (%s)", code, msg)
	}

	return nil
}
