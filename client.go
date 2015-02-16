// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
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
	Store(path string, src io.Reader) error
	Close() error
}

type client struct {
	config       Config
	hosts        []string
	freeConnCh   chan *persistentConn
	numOpenConns int32
	mu           sync.Mutex
	t0           time.Time
	connIdx      int
	closed       bool
	allCons      map[int]*persistentConn
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
		allCons:    make(map[int]*persistentConn),
	}
}

// Closes all open server connections. Currently this does not attempt
// to do any kind of polite FTP connection termination. It will interrupt
// all transfers in progress.
func (c *client) Close() error {
	c.mu.Lock()
	if c.closed {
		return errors.New("already closed")
	}
	c.closed = true

	var cons []*persistentConn
	for _, pconn := range c.allCons {
		cons = append(cons, pconn)
	}
	c.mu.Unlock()

	for _, pconn := range cons {
		c.removeConn(pconn)
	}

	return nil
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
				c.removeConn(pconn)
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
				c.removeConn(pconn)
			} else {
				c.debug("waited for #%d", pconn.idx)
				return pconn, nil
			}
		}
	}
}

func (c *client) removeConn(pconn *persistentConn) {
	c.mu.Lock()
	delete(c.allCons, pconn.idx)
	c.mu.Unlock()
	pconn.close()
}

func (c *client) returnConn(pconn *persistentConn) {
	c.freeConnCh <- pconn
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

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		err = errors.New("client closed")
		goto Error
	}

	c.allCons[idx] = pconn
	return pconn, nil

Error:
	pconn.close()
	return nil, err
}

// fetch SIZE of file
func (c *client) size(path string) int64 {
	pconn, err := c.getIdleConn()
	if err != nil {
		return -1
	}

	defer c.returnConn(pconn)

	if !pconn.hasFeature("SIZE") {
		pconn.debug("server doesn't support SIZE")
		return -1
	}

	pconn.debug("requesting SIZE %s", path)
	code, msg, err := pconn.sendCommand("SIZE %s", path)
	if err != nil {
		pconn.debug("failed running SIZE: %s", err)
		return -1
	}

	if code != ReplyFileStatus {
		pconn.debug("unexpected SIZE response: %d (%s)", code, msg)
		return -1
	} else {
		size, err := strconv.ParseInt(msg, 10, 64)
		if err != nil {
			pconn.debug(`failed parsing SIZE response "%s": %s`, msg, err)
			return -1
		} else {
			return size
		}
	}
}

func (c *client) canResume() bool {
	pconn, err := c.getIdleConn()
	if err != nil {
		return false
	}

	defer c.returnConn(pconn)

	return pconn.hasFeatureWithArg("REST", "STREAM")
}

// Retrieve file "path" from server and write bytes to "dest". If the
// server supports resuming stream transfers, Retrieve will continue
// resuming a failed download as long as it continues making progress.
func (c *client) Retrieve(path string, dest io.Writer) error {
	// fetch file size to check against how much we transferred
	size := c.size(path)

	canResume := c.canResume()

	var bytesSoFar int64
	for {
		n, err := c.transferFromOffset(path, dest, nil, bytesSoFar)

		bytesSoFar += n

		if err == nil {
			break
		} else if n == 0 {
			return err
		} else if !canResume {
			return fmt.Errorf("%s (can't resume)", err)
		}
	}

	if size != -1 && bytesSoFar != size {
		return fmt.Errorf("expected %d bytes, got %d", size, bytesSoFar)
	}

	return nil
}

// Read bytes from "src" and save as file "path" on the server. If the
// server supports resuming stream transfers and "src" is an io.Seeker
// (*os.File is an io.Seeker), Store will continue resuming a failed upload
// as long as it continues making progress. Store will not attempt to
// resume an upload if the client is connected to multiple servers.
func (c *client) Store(path string, src io.Reader) error {

	canResume := len(c.hosts) == 1 && c.canResume()

	seeker, ok := src.(io.Seeker)
	if !ok {
		canResume = false
	}

	var (
		bytesSoFar int64
		err        error
		n          int64
	)
	for {
		if bytesSoFar > 0 {
			size := c.size(path)
			if size == -1 {
				return fmt.Errorf("%s (resume failed)", err)
			}

			_, seekErr := seeker.Seek(size, os.SEEK_SET)
			if seekErr != nil {
				c.debug("failed seeking to %d while resuming upload to %s: %s",
					size+1,
					path,
					err,
				)
				return fmt.Errorf("%s (resume failed)", err)
			}
			bytesSoFar = size
		}

		n, err = c.transferFromOffset(path, nil, src, bytesSoFar)

		bytesSoFar += n

		if err == nil {
			break
		} else if n == 0 {
			return err
		} else if !canResume {
			return fmt.Errorf("%s (can't resume)", err)
		}
	}

	// fetch file size to check against how much we transferred
	size := c.size(path)
	if size != -1 && size != bytesSoFar {
		return fmt.Errorf("sent %d bytes, but size is %d", bytesSoFar, size)
	}

	return nil
}

func (c *client) transferFromOffset(path string, dest io.Writer, src io.Reader, offset int64) (int64, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return 0, err
	}

	defer c.returnConn(pconn)

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
			return 0, fmt.Errorf("server doesn't support resuming")
		}
	}

	dc, err := pconn.openDataConn()
	if err != nil {
		pconn.debug("error opening data connection: %s", err)
		return 0, err
	}

	// to catch early returns
	defer dc.Close()

	var cmd string
	if dest == nil && src != nil {
		dest = dc
		cmd = "STOR"
	} else if dest != nil && src == nil {
		src = dc
		cmd = "RETR"
	} else {
		panic("this shouldn't happen")
	}

	code, msg, err := pconn.sendCommand("%s %s", cmd, path)
	if err != nil {
		return 0, err
	}

	if !positivePreliminaryReply(code) {
		pconn.debug("unexpected response to %s: %d (%s)", cmd, code, msg)
		return 0, fmt.Errorf("unexpected response to %s: %d (%s)", cmd, code, msg)
	}

	n, err := io.Copy(dest, src)

	if err != nil {
		pconn.broken = true
		return n, err
	}

	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}

	code, msg, err = pconn.readResponse(0)
	if err != nil {
		pconn.debug("error reading response after %s: %s", cmd, err)
		return n, err
	}

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected response after %s: %d (%s)", cmd, code, msg)
		return n, fmt.Errorf("unexpected response after %s: %d (%s)", cmd, code, msg)
	}

	return n, nil
}

func (c *client) NameList(path string) ([]string, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return nil, err
	}

	defer c.returnConn(pconn)

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
