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
	"os/exec"
	"path"
	"testing"
	"time"
)

var goftpConfig = Config{
	User:     "goftp",
	Password: "rocks",
}

// list of addresses for tests to connect to
var ftpdAddrs []string

var (
	// used for implicit tls test
	implicitTLSAddrs = []string{"127.0.0.1:2122", "[::1]:2122"}
	pureAddrs        = []string{"127.0.0.1:2121", "[::1]:2121"}
	proAddrs         = []string{"127.0.0.1:2124"}
)

func TestMain(m *testing.M) {
	implicitCloser, err := startPureFTPD(implicitTLSAddrs, "ftpd/pure-ftpd-implicittls")

	if err != nil {
		log.Fatal(err)
	}

	pureCloser, err := startPureFTPD(pureAddrs, "ftpd/pure-ftpd")
	ftpdAddrs = append(ftpdAddrs, pureAddrs...)

	if err != nil {
		log.Fatal(err)
	}

	proCloser, err := startProFTPD()
	// this port is hard coded in its config
	ftpdAddrs = append(ftpdAddrs, proAddrs...)

	if err != nil {
		log.Fatal(err)
	}

	var ret int
	func() {
		defer implicitCloser()
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

		cmd := exec.Command(binary,
			"--bind", host+","+port,
			"--login", "puredb:ftpd/users.pdb",
			"--tls", "1",
		)

		cmd.Env = []string{fmt.Sprintf("FTP_ANON_DIR=%s/testroot", cwd)}

		cmd.Stderr = os.Stderr

		err = cmd.Start()
		if err != nil {
			return nil, fmt.Errorf("error starting pure-ftpd on %s: %s", addr, err)
		}

		ftpdProcs = append(ftpdProcs, cmd.Process)
	}

	closer := func() {
		for _, proc := range ftpdProcs {
			proc.Kill()
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

	cmd := exec.Command(binary,
		"--nodaemon",
		"--config", path.Join(cwd, "ftpd", "proftpd.conf"),
		// "--debug", "10",
	)

	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("error starting proftpd on: %s", err)
	}

	closer := func() {
		cmd.Process.Kill()
	}

	// give it a bit to get started
	time.Sleep(100 * time.Millisecond)

	return closer, nil
}
