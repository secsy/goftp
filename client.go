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
		t0:         time.Now(),
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
	t0           time.Time
}

func (c *client) debug(f string, args ...interface{}) {
	if c.config.Logger == nil {
		return
	}

	msg := fmt.Sprintf("goftp: %.3f %s\n",
		time.Now().Sub(c.t0).Seconds(),
		fmt.Sprintf(f, args...),
	)

	c.config.Logger.Write([]byte(msg))
}

func (c *client) getIdleConn() (*persistentConn, error) {

	// First check for available connections in the channel.
Loop:
	for {
		select {
		case pconn := <-c.freeConnCh:
			if pconn.broken {
				c.debug("#%d was ready (broken)", pconn.idx)
				atomic.AddInt32(&c.numOpenConns, -1)
				pconn.close()
			} else {
				c.debug("#%d was ready", pconn.idx)
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
				c.debug("#%d error connecting: %s", idx, err)
				atomic.AddInt32(&c.numOpenConns, -1)
			}
			return pconn, err
		} else {
			c.mu.Unlock()

			// block waiting for a free connection
			pconn := <-c.freeConnCh

			if pconn.broken {
				c.debug("waited for #%d (broken)", pconn.idx)
				atomic.AddInt32(&c.numOpenConns, -1)
				pconn.close()
			} else {
				c.debug("waited for #%d", pconn.idx)
				return pconn, nil
			}
		}
	}
}

// open control connection and log in if appropriate
func (c *client) openConn(idx int32) (pconn *persistentConn, err error) {
	c.debug("opening #%d to %s", idx, c.config.Host)
	conn, err := net.DialTimeout("tcp", c.config.Host, c.config.Timeout)
	if err != nil {
		return nil, err
	}

	pconn = &persistentConn{
		controlConn: conn,
		idx:         idx,
		reader:      textproto.NewReader(bufio.NewReader(conn)),
		writer:      textproto.NewWriter(bufio.NewWriter(conn)),
		features:    make(map[string]string),
		config:      c.config,
		t0:          c.t0,
	}

	_, _, err = pconn.reader.ReadResponse(ReplyServiceReady)
	if err != nil {
		goto Error
	}

	if err = pconn.logInConn(); err != nil {
		goto Error
	}

	if err = pconn.fetchFeatures(); err != nil {
		goto Error
	}

	return pconn, nil

Error:
	pconn.close()
	return nil, err
}

func (c *client) Retrieve(path string, dest io.Writer) error {
	pconn, err := c.getIdleConn()
	if err != nil {
		return err
	}

	defer func() {
		c.freeConnCh <- pconn
	}()

	if err = pconn.setType("I"); err != nil {
		return err
	}

	dc, err := pconn.openDataConn()
	if err != nil {
		pconn.debug("error opening data connection: %s", err)
		return err
	}

	// to catch early returns
	defer dc.Close()

	pconn.debug("sending RETR %s", path)
	code, msg, err := pconn.sendCommand("RETR %s", path)
	if err != nil {
		return err
	}

	if !positivePreliminaryReply(code) {
		pconn.debug("unexpected response: %d (%s)", code, msg)
		return fmt.Errorf("unexpected response: %d (%s)", code, msg)
	}

	_, err = io.Copy(dest, dc)

	if err != nil {
		return err
	}

	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}

	code, msg, err = pconn.reader.ReadResponse(0)
	if err != nil {
		pconn.debug("error reading response: %s", err)
		return err
	}

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected result: %d (%s)", code, msg)
		return fmt.Errorf("unexpected result: %d (%s)", code, msg)
	}

	return nil
}

func (c *client) NameList(path string) ([]string, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return nil, err
	}

	defer func() {
		c.freeConnCh <- pconn
	}()

	dc, err := pconn.openDataConn()
	if err != nil {
		pconn.debug("error opening data connection: %s", err)
		return nil, err
	}

	// to catch early returns
	defer dc.Close()

	pconn.debug("sending NLST")
	code, msg, err := pconn.sendCommand("NLST %s", path)
	if err != nil {
		return nil, err
	}

	if !positivePreliminaryReply(code) {
		pconn.debug("unexpected response: %d (%s)", code, msg)
		return nil, fmt.Errorf("unexpected response: %d (%s)", code, msg)
	}

	scanner := bufio.NewScanner(dc)
	scanner.Split(bufio.ScanLines)

	var res []string
	for scanner.Scan() {
		res = append(res, scanner.Text())
	}

	var dataError error
	if err = scanner.Err(); err != nil {
		pconn.debug("error reading NLST data: %s", err)
		dataError = fmt.Errorf("error reading NLST data: %s", err)
	}

	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}

	code, msg, err = pconn.reader.ReadResponse(0)
	if err != nil {
		pconn.debug("error reading response: %s", err)
		return nil, err
	}

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected result: %d (%s)", code, msg)
		return nil, fmt.Errorf("unexpected result: %d (%s)", code, msg)
	}

	pconn.debug("finished NameList (%d %s)", code, msg)

	if dataError != nil {
		return nil, dataError
	}

	return res, nil
}
