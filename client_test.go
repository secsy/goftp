// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"crypto/tls"
	"testing"
	"time"
)

func TestTimeoutConnect(t *testing.T) {
	config := Config{Timeout: 100 * time.Millisecond}

	c, err := DialConfig(config, "168.254.111.222:2121")

	t0 := time.Now()
	_, err = c.NameList("")
	delta := time.Now().Sub(t0)

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

	if int(c.numOpenConns) != len(c.freeConnCh) {
		t.Error("Leaked a connection")
	}
}

func TestExplicitTLS(t *testing.T) {
	for _, addr := range ftpdAddrs[2:] {
		config := Config{
			User:     "goftp",
			Password: "rocks",
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

		if int(c.numOpenConns) != len(c.freeConnCh) {
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

		if int(c.numOpenConns) != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}
