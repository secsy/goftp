// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"sync"
	"testing"
	"time"
)

func TestTimeoutConnect(t *testing.T) {
	config := Config{Timeout: 100 * time.Millisecond}

	c, err := DialConfig(config, "168.254.111.222:2121")

	t0 := time.Now()
	_, err = c.ReadDir("")
	delta := time.Since(t0)

	if err == nil || !err.(Error).Temporary() {
		t.Error("Expected a timeout error")
	}

	offBy := delta - config.Timeout
	if offBy < 0 {
		offBy = -offBy
	}
	if offBy > 50*time.Millisecond {
		t.Errorf("Timeout of 100ms was off by %s", offBy)
	}

	if c.numOpenConns() != len(c.freeConnCh) {
		t.Error("Leaked a connection")
	}
}

func TestExplicitTLS(t *testing.T) {
	for _, addr := range ftpdAddrs {
		var gotAddr string
		config := Config{
			User:     "goftp",
			Password: "rocks",
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if gotAddr == "" {
					gotAddr = addr
				}
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSMode: TLSExplicit,
		}

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)
		err = c.Retrieve("subdir/1234.bin", buf)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal([]byte{1, 2, 3, 4}, buf.Bytes()) {
			t.Errorf("Got %v", buf.Bytes())
		}

		if gotAddr != addr {
			t.Errorf("Expected dial to %s, got %s", addr, gotAddr)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestImplicitTLS(t *testing.T) {
	closer, err := startPureFTPD(implicitTLSAddrs, "ftpd/pure-ftpd-implicittls")
	if err != nil {
		t.Fatal(err)
	}

	defer closer()

	for _, addr := range implicitTLSAddrs {
		config := Config{
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSMode: TLSImplicit,
		}

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)
		err = c.Retrieve("subdir/1234.bin", buf)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal([]byte{1, 2, 3, 4}, buf.Bytes()) {
			t.Errorf("Got %v", buf.Bytes())
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestPooling(t *testing.T) {
	config := Config{
		ConnectionsPerHost: 2,
		User:               "goftp",
		Password:           "rocks",
	}
	c, err := DialConfig(config, ftpdAddrs...)
	if err != nil {
		t.Fatal(err)
	}

	wg := sync.WaitGroup{}
	ok := true
	numConns := config.ConnectionsPerHost * len(ftpdAddrs)

	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func() {
			buf := new(bytes.Buffer)
			err := c.Retrieve("subdir/1234.bin", buf)
			if err != nil || !bytes.Equal(buf.Bytes(), []byte{1, 2, 3, 4}) {
				ok = false
			}
			wg.Done()
		}()
	}

	wg.Wait()

	if !ok {
		t.Error("something went wrong")
	}

	if len(c.freeConnCh) != numConns {
		t.Errorf("Expected %d conns, was %d", numConns, len(c.freeConnCh))
	}

	if c.numOpenConns() != len(c.freeConnCh) {
		t.Error("Leaked a connection")
	}
}
