// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"testing"
	"time"
)

// list of addresses for tests to connect to
var ftpdAddrs []string

// used for implicit tls test
var implicitTLSAddrs = []string{"127.0.0.1:2122", "[::1]:2122"}

var goftpConfig = Config{
	User:     "goftp",
	Password: "rocks",
}

func TestMain(m *testing.M) {
	pureAddrs := []string{"127.0.0.1:2121", "[::1]:2121"}
	pureCloser, err := startPureFTPD(pureAddrs, "ftpd/pure-ftpd")
	ftpdAddrs = append(ftpdAddrs, pureAddrs...)

	if err != nil {
		log.Fatal(err)
	}

	proCloser, err := startProFTPD()
	// this port is hard coded in its config
	ftpdAddrs = append(ftpdAddrs, "127.0.0.1:2123")

	if err != nil {
		log.Fatal(err)
	}

	var ret int
	func() {
		defer pureCloser()
		defer proCloser()
		ret = m.Run()
	}()

	os.Exit(ret)
}

func startPureFTPD(addrs []string, binary string) (func(), error) {
	if _, err := os.Open("client_test.go"); os.IsNotExist(err) {
		return nil, errors.New("must run tests in goftp/ directory")
	}

	if _, err := os.Stat(binary); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s not found - you need to run ./build_test_server.sh from the goftp directory", binary)
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
			binary,
			[]string{binary,
				"--bind", host + "," + port,
				"--login", "puredb:ftpd/users.pdb",
				"--tls", "1",
			},
			&os.ProcAttr{
				Env:   []string{fmt.Sprintf("FTP_ANON_DIR=%s/testroot", cwd)},
				Files: []*os.File{os.Stdin, os.Stderr, os.Stderr},
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

// ./proftpd --nodaemon --config `pwd`/proftpd.conf
func startProFTPD() (func(), error) {
	if _, err := os.Open("client_test.go"); os.IsNotExist(err) {
		return nil, errors.New("must run tests in goftp/ directory")
	}

	binary := "ftpd/proftpd"

	if _, err := os.Stat(binary); os.IsNotExist(err) {
		return nil, fmt.Errorf("%s not found - you need to run ./build_test_server.sh from the goftp directory", binary)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("couldn't determine cwd: %s", err)
	}

	ftpdProc, err := os.StartProcess(
		binary,
		[]string{binary,
			"--nodaemon",
			"--config", path.Join(cwd, "ftpd", "proftpd.conf"),
		},
		&os.ProcAttr{
			Files: []*os.File{os.Stdin},
		},
	)

	if err != nil {
		return nil, fmt.Errorf("error starting proftpd on: %s", err)
	}

	closer := func() {
		ftpdProc.Signal(os.Interrupt)
		ftpdProc.Wait()
	}

	// give it a bit to get started
	time.Sleep(100 * time.Millisecond)

	return closer, nil
}
