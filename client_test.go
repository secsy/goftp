// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// addresses pure-ftpd will listen on for tests to use
var ftpdAddrs = []string{"127.0.0.1:2121", "[::1]:2121"}

func TestMain(m *testing.M) {
	closer, err := startPureFTPD(ftpdAddrs)

	if err != nil {
		log.Fatal(err)
	}

	var ret int
	func() {
		defer closer()
		ret = m.Run()
	}()

	os.Exit(ret)
}

// start instance of pure-ftpd for each listn addr in ftpdAddrs
func startPureFTPD(addrs []string) (func(), error) {
	if _, err := os.Open("client_test.go"); os.IsNotExist(err) {
		return nil, errors.New("must run tests in goftp/ directory")
	}

	if _, err := os.Open("ftpd/pure-ftpd"); os.IsNotExist(err) {
		return nil, errors.New("pure-ftpd not found! You need to run ./build_test_server.sh from the goftp directory.")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("couldn't determine cwd: %s", err)
	}

	var ftpdProcs []*os.Process
	for _, addr := range addrs {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			panic(err)
		}

		ftpdProc, err := os.StartProcess(
			"ftpd/pure-ftpd",
			[]string{"ftpd/pure-ftpd", "--bind", host + "," + port, "--login", "puredb:ftpd/users.pdb"},
			&os.ProcAttr{
				Env:   []string{fmt.Sprintf("FTP_ANON_DIR=%s/testroot", cwd)},
				Files: []*os.File{nil, os.Stderr, os.Stderr},
			},
		)

		if err != nil {
			return nil, fmt.Errorf("error starting pure-ftpd on %s: %s", addr, err)
		}

		ftpdProcs = append(ftpdProcs, ftpdProc)
	}

	closer := func() {
		for _, proc := range ftpdProcs {
			proc.Signal(os.Interrupt)
			proc.Wait()
		}
	}

	// give them a bit to get started
	time.Sleep(100 * time.Millisecond)

	return closer, nil
}

func TestNameList(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := Dial(addr)

		if err != nil {
			t.Fatal(err)
		}

		list, err := c.NameList("")

		if err != nil {
			t.Fatal(err)
		}

		sort.Strings(list)

		if !reflect.DeepEqual([]string{"git-ignored", "lorem.txt", "subdir"}, list) {
			t.Errorf("Got %v", list)
		}

		list, err = c.NameList("subdir")

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual([]string{"1234.bin"}, list) {
			t.Errorf("Got %v", list)
		}
	}
}

func TestRetrieve(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := Dial(addr)

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
		c, err := Dial(addr)

		if err != nil {
			t.Fatal(err)
		}

		buf := new(testWriter)

		buf.cb = func(p []byte) (int, error) {
			if len(p) <= 2 {
				return len(p), nil
			} else {
				return 2, errors.New("too many bytes to handle!")
			}
		}

		err = c.Retrieve("subdir/1234.bin", buf)

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual([][]byte{[]byte{1, 2}, []byte{3, 4}}, buf.writes) {
			t.Errorf("Got %v", buf.writes)
		}
	}
}

// In this test we simulate a read error by closing all connections
// part way through the download.
func TestResumeRetrieveOnReadError(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := Dial(addr)

		if err != nil {
			t.Fatal(err)
		}

		buf := new(testWriter)

		buf.cb = func(p []byte) (int, error) {
			if len(p) <= 2 {
				return len(p), nil
			} else {
				// close all the connections, then reset closed so we
				// can keep using this client
				c.Close()
				c.(*client).closed = false
				return 2, errors.New("too many bytes to handle!")
			}
		}

		err = c.Retrieve("subdir/1234.bin", buf)

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual([][]byte{[]byte{1, 2}, []byte{3, 4}}, buf.writes) {
			t.Errorf("Got %v", buf.writes)
		}
	}
}

func TestTimeoutConnect(t *testing.T) {
	config := Config{Timeout: 100 * time.Millisecond}

	c, err := DialConfig(config, "168.254.111.222:2121")

	t0 := time.Now()
	_, err = c.NameList("")
	delta := time.Now().Sub(t0)

	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Error("Expected a timeout error")
	}

	offBy := delta - config.Timeout
	if offBy < 0 {
		offBy = -offBy
	}
	if offBy > 50*time.Millisecond {
		t.Errorf("Timeout of 100ms was off by %s", offBy)
	}
}

func TestStore(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := Dial(addr)

		if err != nil {
			t.Fatal(err)
		}

		toSend, err := os.Open("testroot/subdir/1234.bin")
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foo")

		err = c.Store("/git-ignored/foo", toSend)

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
	for _, addr := range ftpdAddrs[0:1] {
		// pure-ftpd doesn't let anonymous users write to existing files,
		// so we use a separate user to test resuming uploads
		config := Config{
			User:     "goftp",
			Password: "rocks",
		}
		c, err := DialConfig(config, addr)

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
					c.Close()
					c.(*client).closed = false
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
	}
}
