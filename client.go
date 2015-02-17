// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type TLSMode int

const (
	TLSExplicit TLSMode = 0
	TLSImplicit TLSMode = 1
)

// Client configuration object.
type Config struct {
	// User name. Defaults to "anonymous".
	User string

	// User password. Defaults to "anonymous" if required.
	Password string

	// Maximum number of FTP connections to open at once. Defaults to 5.
	MaxConnections int32

	// Timeout for opening connections and sending individual commands. Defaults
	// to 5 seconds.
	Timeout time.Duration

	// TLS Config used for FTPS. If provided, it will be an error if the server
	// does not support TLS. Both the control and data connection will use TLS.
	TLSConfig *tls.Config

	// FTPS mode. TLSExplicit means connect non-TLS, then upgrade connection to
	// TLS via "AUTH TLS" command. TLSImplicit means open the connection using
	// TLS. Defaults to TLSExplicit.
	TLSMode TLSMode

	// Logging destination for debugging messages. Set to os.Stderr to log to stderr.
	Logger io.Writer
}

type Client struct {
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
func newClient(config Config, hosts []string) *Client {

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

	return &Client{
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
func (c *Client) Close() error {
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
func (c *Client) debug(f string, args ...interface{}) {
	if c.config.Logger == nil {
		return
	}

	msg := fmt.Sprintf("goftp: %.3f %s\n",
		time.Now().Sub(c.t0).Seconds(),
		fmt.Sprintf(f, args...),
	)

	c.config.Logger.Write([]byte(msg))
}

// Get an idle connection.
func (c *Client) getIdleConn() (*persistentConn, error) {

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
				c.debug("waited and got #%d (broken)", pconn.idx)
				atomic.AddInt32(&c.numOpenConns, -1)
				c.removeConn(pconn)
			} else {
				c.debug("waited and got #%d", pconn.idx)
				return pconn, nil
			}
		}
	}
}

func (c *Client) removeConn(pconn *persistentConn) {
	c.mu.Lock()
	delete(c.allCons, pconn.idx)
	c.mu.Unlock()
	pconn.close()
}

func (c *Client) returnConn(pconn *persistentConn) {
	c.freeConnCh <- pconn
}

// Open and set up a control connection.
func (c *Client) openConn(idx int) (pconn *persistentConn, err error) {
	pconn = &persistentConn{
		idx:      idx,
		features: make(map[string]string),
		config:   c.config,
		t0:       c.t0,
	}

	host := c.hosts[idx%len(c.hosts)]

	var conn net.Conn

	if c.config.TLSConfig != nil && c.config.TLSMode == TLSImplicit {
		pconn.debug("opening TLS control connection to %s", host)
		dialer := &net.Dialer{
			Timeout: c.config.Timeout,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", host, pconn.config.TLSConfig)
	} else {
		pconn.debug("opening control connection to %s", host)
		conn, err = net.DialTimeout("tcp", host, c.config.Timeout)
	}

	if err != nil {
		return nil, err
	}

	pconn.setControlConn(conn)

	_, _, err = pconn.readResponse(replyServiceReady)
	if err != nil {
		goto Error
	}

	if c.config.TLSConfig != nil && c.config.TLSMode == TLSExplicit {
		err = pconn.logInTLS()
		if err != nil {
			goto Error
		}
	} else {
		if err = pconn.logIn(); err != nil {
			goto Error
		}
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
