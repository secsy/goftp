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
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	closer, err := startPureFTPD()

	if err != nil {
		log.Fatal(err)
	}

	// give it a bit to get started
	time.Sleep(100 * time.Millisecond)

	var ret int
	func() {
		defer closer()
		ret = m.Run()
	}()

	os.Exit(ret)
}

// port that the test ftp server listens on
var ftpdPort = "2121"

// startup up a test ftp server for each of these addresses
// not sure if ::1 will works on all systems
var ftpdAddrs = []string{"127.0.0.1", "::1"}

func startPureFTPD() (func(), error) {
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
	for _, addr := range ftpdAddrs {
		ftpdProc, err := os.StartProcess(
			"ftpd/pure-ftpd",
			[]string{"--bind", addr + "," + ftpdPort},
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
