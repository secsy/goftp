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
	"strconv"
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

	// Maximum number of FTP connections to open at once. Defaults to 5.
	MaxConnections int32

	// Timeout for opening connections and sending individual commands. Defaults to
	// 5 seconds.
	Timeout time.Duration

	// Logging destination for debugging messages. Set to os.Stderr to log to stderr.
	Logger io.Writer
}

// Client connection interface.
type Conn interface {
	Retrieve(path string, dest io.Writer) error
	NameList(path string) ([]string, error)
}

type client struct {
	config       Config
	hosts        []string
	freeConnCh   chan *persistentConn
	numOpenConns int32
	mu           sync.Mutex
	t0           time.Time
	connIdx      int
}

// Construct and return a new client Conn, setting default config
// values as necessary.
func newClient(config Config, hosts []string) Conn {

	if config.MaxConnections <= 0 {
		config.MaxConnections = 5
	}

	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
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
		hosts:      hosts,
	}
}

// Log a debug message in the context of the client (i.e. not for a
// particular connection).
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
			c.connIdx++
			idx := c.connIdx
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
func (c *client) openConn(idx int) (pconn *persistentConn, err error) {
	host := c.hosts[idx%len(c.hosts)]
	c.debug("#%d dialing %s", idx, host)
	conn, err := net.DialTimeout("tcp", host, c.config.Timeout)
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

	_, _, err = pconn.readResponse(ReplyServiceReady)
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

// Retrieve file "path" from server and write bytes to "dest". If the
// server supports resuming stream transfers, Retrieve will continue
// resuming a failed download as long as it continues making progress.
func (c *client) Retrieve(path string, dest io.Writer) error {
	pconn, err := c.getIdleConn()
	if err != nil {
		return err
	}

	var (
		size    int64
		gotSize bool
	)

	if pconn.hasFeature("SIZE") {
		pconn.debug("requesting SIZE %s", path)
		code, msg, err := pconn.sendCommand("SIZE %s", path)
		if err != nil {
			pconn.debug("failed running SIZE: %s", err)
			c.freeConnCh <- pconn
			return err
		}

		if code != ReplyFileStatus {
			pconn.debug("unexpected SIZE response: %d (%s)", code, msg)
		} else {
			size, err = strconv.ParseInt(msg, 10, 64)
			if err != nil {
				pconn.debug(`failed parsing SIZE response "%s": %s`, msg, err)
			} else {
				gotSize = true
			}
		}
	}

	c.freeConnCh <- pconn

	var bytesSoFar int64
	hasResume := pconn.hasFeatureWithArg("REST", "STREAM")
	for {
		n, err := c.retrieveAtOffset(path, dest, bytesSoFar)

		bytesSoFar += n

		if err == nil {
			break
		} else if n == 0 || !hasResume {
			return err
		}
	}

	if gotSize && bytesSoFar != size {
		return fmt.Errorf("expected %d bytes, got %d", size, bytesSoFar)
	}

	return nil
}

func (c *client) retrieveAtOffset(path string, dest io.Writer, offset int64) (int64, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return 0, err
	}

	defer func() {
		c.freeConnCh <- pconn
	}()

	if err = pconn.setType("I"); err != nil {
		return 0, err
	}

	if offset > 0 {
		pconn.debug("requesting REST %d", offset)
		code, msg, err := pconn.sendCommand("REST %d", offset)
		if err != nil {
			return 0, err
		}

		if code != ReplyFileActionPending {
			pconn.debug("unexpected response to REST: %d (%s)", code, msg)
			return 0, fmt.Errorf("failed resuming download (%s)", msg)
		}
	}

	dc, err := pconn.openDataConn()
	if err != nil {
		pconn.debug("error opening data connection: %s", err)
		return 0, err
	}

	// to catch early returns
	defer dc.Close()

	pconn.debug("sending RETR %s", path)
	code, msg, err := pconn.sendCommand("RETR %s", path)
	if err != nil {
		return 0, err
	}

	if !positivePreliminaryReply(code) {
		pconn.debug("unexpected response to RETR: %d (%s)", code, msg)
		return 0, fmt.Errorf("unexpected response: %d (%s)", code, msg)
	}

	n, err := io.Copy(dest, dc)

	if err != nil {
		// not sure what state this connection is in (e.g. if we got a write
		// error, the server might still think the transfer completed)
		pconn.broken = true
		return n, err
	}

	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}

	code, msg, err = pconn.readResponse(0)
	if err != nil {
		pconn.debug("error reading response after RETR: %s", err)
		return n, err
	}

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected response after RETR: %d (%s)", code, msg)
		return n, fmt.Errorf("unexpected response after RETR: %d (%s)", code, msg)
	}

	return n, nil
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

	code, msg, err = pconn.readResponse(0)
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
