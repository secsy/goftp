// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Client config
type Config struct {
	// User name. Defaults to "anonymous"
	User string

	// User password. Defaults to "anonymous" if required.
	Password string

	// User account.
	Account string

	// FTP server (e.g. "ftp.example.com" or "ftp.example.com:1337"). Port will default
	// to 21.
	Host string

	// Maximum number of FTP connections to open at once. Defaults to 10.
	MaxConnections int32

	// Timeout for opening connections and sending individual commands. Defaults to
	// 5 seconds.
	Timeout time.Duration

	// Logging destination for debugging messages. Set to os.Stderr to log to stderr.
	Logger io.Writer
}

// Represents a single connection to an FTP server.
type persistentConn struct {
	// control socket
	controlConn net.Conn

	// data socket
	dataConn net.Conn

	// control socket read/write helpers
	reader *textproto.Reader
	writer *textproto.Writer

	// has this connection encountered an unrecoverable error
	broken bool

	// index from 1..MaxConnections (used for logging context)
	idx int32
}

func (pconn *persistentConn) close() {
	if pconn.controlConn != nil {
		pconn.controlConn.Close()
	}
	if pconn.dataConn != nil {
		pconn.dataConn.Close()
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

// Construct and return a new client Conn
func NewClient(config Config) Conn {

	if config.MaxConnections <= 0 {
		config.MaxConnections = 10
	}

	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}

	if strings.Index(config.Host, ":") == -1 {
		config.Host = config.Host + ":21"
	}

	if config.User == "" {
		config.User = "anonymous"
	}

	if config.Password == "" {
		config.Password = "anonymous"
	}

	return &client{
		config:     config,
		freeConnCh: make(chan *persistentConn, config.MaxConnections),
	}
}

// Client connection interface.
type Conn interface {
	Retrieve(path string, dest io.Writer) error
	NameList(path string) ([]string, error)
}

type client struct {
	config       Config
	freeConnCh   chan *persistentConn
	numOpenConns int32
	mu           sync.Mutex
}

// pass along error so you can chain to return
func (c *client) debug(f string, args ...interface{}) {
	if c.config.Logger == nil {
		return
	}

	c.config.Logger.Write([]byte(fmt.Sprintf("goftp: "+f+"\n", args...)))
}

func (c *client) getIdleConn() (*persistentConn, error) {

	// First check for available connections in the channel.
Loop:
	for {
		select {
		case pconn := <-c.freeConnCh:
			if pconn.broken {
				c.debug("connection %d was waiting (broken)", pconn.idx)
				atomic.AddInt32(&c.numOpenConns, -1)
				pconn.close()
			} else {
				c.debug("connection %d was waiting", pconn.idx)
				return pconn, nil
			}
		default:
			break Loop
		}
	}

	// No available connections. Loop until we can open a new one, or
	// one becomes available.
	for {
		c.mu.Lock()
		if c.numOpenConns < c.config.MaxConnections {
			c.numOpenConns++
			idx := c.numOpenConns
			c.mu.Unlock()
			pconn, err := c.openConn(idx)
			if err != nil {
				c.debug("connection %d error connecting: %s", idx, err)
				atomic.AddInt32(&c.numOpenConns, -1)
			}
			return pconn, err
		} else {
			c.mu.Unlock()

			// block waiting for a free connection
			pconn := <-c.freeConnCh

			if pconn.broken {
				c.debug("waited for connection %d (broken)", pconn.idx)
				atomic.AddInt32(&c.numOpenConns, -1)
				pconn.close()
			} else {
				c.debug("waited for connection %d", pconn.idx)
				return pconn, nil
			}
		}
	}
}

// open control connection and log in if appropriate
func (c *client) openConn(idx int32) (pconn *persistentConn, err error) {
	c.debug("connection %d dialing %s", idx, c.config.Host)
	conn, err := net.Dial("tcp", c.config.Host)
	if err != nil {
		return nil, err
	}

	pconn = &persistentConn{
		controlConn: conn,
		idx:         idx,
		reader:      textproto.NewReader(bufio.NewReader(conn)),
		writer:      textproto.NewWriter(bufio.NewWriter(conn)),
	}

	_, _, err = pconn.reader.ReadResponse(ReplyServiceReady)
	if err != nil {
		goto Error
	}

	if err = c.logInConn(pconn); err != nil {
		goto Error
	}

	c.debug("connection %d finished setup", idx)

	return pconn, nil

Error:
	pconn.close()
	return nil, err
}

func (c *client) logInConn(pconn *persistentConn) error {
	if c.config.User == "" {
		return nil
	}

	c.debug("connection %d logging in as user %s", pconn.idx, c.config.User)
	code, msg, err := pconn.sendCommand("USER %s", c.config.User)
	if err != nil {
		return err
	}

	if code == ReplyNeedPassword {
		c.debug("connection %d sending password", pconn.idx)
		code, msg, err = pconn.sendCommand("PASS %s", c.config.Password)
		if err != nil {
			return err
		}
	}

	if !positiveCompletionReply(code) {
		return fmt.Errorf("unexpected response: %d (%s)", code, msg)
	}

	return nil
}

func (c *client) Retrieve(path string, dest io.Writer) error {
	return nil
}

func (c *client) NameList(path string) ([]string, error) {
	c.getIdleConn()
	return nil, nil
}
