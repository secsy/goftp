// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// port that the test ftp server listens on
var ftpdPort = "2121"

// second port some tests use when they want to start their
// own ftpd (e.g. to test error handling when ftpd restarts)
var secondFtpdPort = "2122"

// startup up a test ftp server for each of these addresses
// not sure if ::1 will works on all systems
var ftpdAddrs = []string{"127.0.0.1", "::1"}

func TestMain(m *testing.M) {
	closer, err := startPureFTPD(ftpdAddrs, ftpdPort)

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

func startPureFTPD(addrs []string, port string) (func(), error) {
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
		ftpdProc, err := os.StartProcess(
			"ftpd/pure-ftpd",
			[]string{"--bind", addr + "," + port},
			&os.ProcAttr{
				Env: []string{fmt.Sprintf("FTP_ANON_DIR=%s/testroot", cwd)},
			},
		)

		if err != nil {
			return nil, fmt.Errorf("error starting pure-ftpd on %s:%s: %s", addr, ftpdPort, err)
		}

		ftpdProcs = append(ftpdProcs, ftpdProc)
	}

	closer := func() {
		for _, proc := range ftpdProcs {
			proc.Kill()
			proc.Wait()
		}
	}

	// give it a bit to get started
	time.Sleep(100 * time.Millisecond)

	return closer, nil
}

func TestNameList(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := Dial(fmt.Sprintf("[%s]:%s", addr, ftpdPort))

		if err != nil {
			t.Fatal(err)
		}

		list, err := c.NameList("")

		if err != nil {
			t.Fatal(err)
		}

		sort.Strings(list)

		if !reflect.DeepEqual([]string{"lorem.txt", "subdir"}, list) {
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
		c, err := Dial(fmt.Sprintf("[%s]:%s", addr, ftpdPort))

		if err != nil {
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)

		// first try a file that doesn't to make sure we get an error and our
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

// io.Writer tests use to simulate various exceptional cases during
// file downloads
type twoByter struct {
	writes [][]byte
	cb     func([]byte) (int, error)
}

func (tb *twoByter) Write(p []byte) (int, error) {
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
		c, err := Dial(fmt.Sprintf("[%s]:%s", addr, ftpdPort))

		if err != nil {
			t.Fatal(err)
		}

		buf := new(twoByter)

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

// kill server part way through download
func TestResumeRetrieveOnReadError(t *testing.T) {
	closer, err := startPureFTPD(ftpdAddrs, secondFtpdPort)
	if err != nil {
		t.Fatal(err)
	}

	// not "defer closer()" intentionally
	defer func() {
		closer()
	}()

	for _, addr := range ftpdAddrs {
		c, err := Dial(fmt.Sprintf("[%s]:%s", addr, ftpdPort))

		if err != nil {
			t.Fatal(err)
		}

		buf := new(twoByter)

		buf.cb = func(p []byte) (int, error) {
			if len(p) <= 2 {
				return len(p), nil
			} else {
				// restart the ftpd server
				closer()
				closer, err = startPureFTPD(ftpdAddrs, secondFtpdPort)
				if err != nil {
					t.Fatal(err)
				}
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

	c, err := DialConfig(config, fmt.Sprintf("168.254.111.222:%s", ftpdPort))

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
