// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"errors"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestRetrieve(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)

		// first try a file that doesn't exit to make sure we get an error and our
		// connection is still okay
		err = c.Retrieve("doesnt-exist", buf)

		if err == nil {
			t.Errorf("Expected error about not existing")
		}

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

// io.Writer used to simulate various exceptional cases during
// file downloads
type testWriter struct {
	writes [][]byte
	cb     func([]byte) (int, error)
}

func (tb *testWriter) Write(p []byte) (int, error) {
	n, err := tb.cb(p)
	if n > 0 {
		tb.writes = append(tb.writes, p[0:n])
	}
	return n, err
}

// pure-ftpd supports "REST STREAM", so make sure we can resume downloads.
// In this test we are simulating a client write error.
func TestResumeRetrieveOnWriteError(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		buf := new(testWriter)

		buf.cb = func(p []byte) (int, error) {
			if len(p) <= 2 {
				return len(p), nil
			}
			return 2, errors.New("too many bytes to handle")
		}

		err = c.Retrieve("subdir/1234.bin", buf)

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual([][]byte{[]byte{1, 2}, []byte{3, 4}}, buf.writes) {
			t.Errorf("Got %v", buf.writes)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

// In this test we simulate a read error by closing all connections
// part way through the download.
func TestResumeRetrieveOnReadError(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		buf := new(testWriter)

		buf.cb = func(p []byte) (int, error) {
			if len(p) <= 2 {
				return len(p), nil
			}
			// close all the connections, then reset closed so we
			// can keep using this client
			c.Close()
			c.closed = false
			return 2, errors.New("too many bytes to handle")
		}

		err = c.Retrieve("subdir/1234.bin", buf)

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual([][]byte{[]byte{1, 2}, []byte{3, 4}}, buf.writes) {
			t.Errorf("Got %v", buf.writes)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestStore(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		toSend, err := os.Open("testroot/subdir/1234.bin")
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foo")

		err = c.Store("git-ignored/foo", toSend)

		if err != nil {
			t.Fatal(err)
		}

		stored, err := ioutil.ReadFile("testroot/git-ignored/foo")
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal([]byte{1, 2, 3, 4}, stored) {
			t.Errorf("Got %v", stored)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

// io.Reader that also implements io.Seeker interface like
// *os.File (used to test resuming uploads)
type testSeeker struct {
	buf   *bytes.Reader
	soFar int
	cb    func(int)
}

func (ts *testSeeker) Read(p []byte) (int, error) {
	n, err := ts.buf.Read(p)
	ts.soFar += n
	ts.cb(ts.soFar)
	return n, err
}

func (ts *testSeeker) Seek(offset int64, whence int) (int64, error) {
	return ts.buf.Seek(offset, whence)
}

func randomBytes(b []byte) {
	for i := 0; i < len(b); i++ {
		b[i] = byte(rand.Int31n(256))
	}
}

// kill connections part way through upload - show we can restart if src
// is an io.Seeker
func TestResumeStoreOnWriteError(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		// 10MB of random data
		buf := make([]byte, 10*1024*1024)
		randomBytes(buf)

		closed := false

		seeker := &testSeeker{
			buf: bytes.NewReader(buf),
			cb: func(readSoFar int) {
				if readSoFar > 5*1024*1024 && !closed {
					// close all connections half way through upload

					// if you don't wait a bit here, proftpd deletes the
					// partially uploaded file for some reason
					time.Sleep(100 * time.Millisecond)

					c.Close()
					c.closed = false
					closed = true
				}
			},
		}

		os.Remove("testroot/git-ignored/big")

		err = c.Store("git-ignored/big", seeker)

		if err != nil {
			t.Fatal(err)
		}

		stored, err := ioutil.ReadFile("testroot/git-ignored/big")
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(buf, stored) {
			t.Errorf("buf was %d, stored was %d", len(buf), len(stored))
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}
