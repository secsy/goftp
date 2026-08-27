package goftp

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

// testTimeout replaces the package's 5-second default for the suite.
//
// Five seconds was ample when the servers were processes on the same
// machine. They are containers now, reached over a bridge network that
// on macOS sits inside a virtual machine, and a handful of tests were
// failing intermittently on exactly that boundary — different ones each
// run, every failure at almost exactly five seconds.
//
// A test timing out is not the same as a test failing, and a suite that
// does the first while reporting the second is worse than a slow one.
var testTimeout = 30 * time.Second

var goftpConfig = Config{
	User:     "goftp",
	Password: "rocks",
	Timeout:  testTimeout,
}

// Addresses the suite connects to. They are served by the containers in
// test-servers/, one per implementation, and are the same ports the
// suite has always used.
// They are overridable because where the servers are depends on where
// the tests run. Inside the compose network they are container names;
// from the host they are forwarded ports. FTP makes that distinction
// matter more than most protocols: a passive transfer has the server
// name an address for the client to dial, so a server that believes it
// is at one address while the client reaches it at another cannot
// transfer anything.
var (
	// pure-ftpd, explicit TLS: connect in the clear, then AUTH TLS.
	pureAddrs = addrsFromEnv("GOFTP_PURE_ADDR", "127.0.0.1:2121")
	// pure-ftpd built with implicit TLS: TLS from the first byte.
	implicitTLSAddrs = addrsFromEnv("GOFTP_IMPLICIT_TLS_ADDR", "127.0.0.1:2122")
	// proftpd, a second implementation — a client that only ever meets
	// one server learns that server's habits rather than the protocol's.
	proAddrs = addrsFromEnv("GOFTP_PRO_ADDR", "127.0.0.1:2124")
)

func addrsFromEnv(key, fallback string) []string {
	if v := os.Getenv(key); v != "" {
		return []string{v}
	}
	return []string{fallback}
}

// Whichever of the above answered when the suite started. Tests iterate
// this so they run against every implementation that is up.
var ftpdAddrs []string

// True when nothing was listening. Tests needing a server call
// requireServers and are skipped, rather than failing on a connection
// refused — which reads like a bug in the package instead of a missing
// prerequisite.
var skipServers bool

// requireServers skips the calling test when no FTP server is running.
func requireServers(t *testing.T) {
	t.Helper()
	if skipServers {
		t.Skip("no FTP test server is listening; start them with ./scripts/test-servers.sh up")
	}
}

// listening reports whether something accepts a connection at addr.
func listening(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// waitFor gives a server a moment to come up, for the case where the
// tests are started immediately after the containers.
func waitFor(addr string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for {
		if listening(addr) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestMain finds the servers rather than starting them.
//
// They used to be compiled on whatever machine ran the tests, by a
// script that fetched 2015 tarballs over plain FTP and patched their C —
// and TestMain then spawned the binaries it had built. That made the
// suite unrunnable without building two FTP servers first, and it meant
// the versions under test were whatever that script last managed to
// compile.
//
// They are containers now, described in test-servers/. This connects to
// them. The consequence worth knowing: the suite no longer controls the
// servers' lifetime, so a test must not leave a server in a state the
// next one cannot use.
func TestMain(m *testing.M) {
	// An explicit escape hatch, for running only the tests that need no
	// server at all.
	if os.Getenv("GOFTP_SKIP_SERVERS") != "" {
		skipServers = true
		os.Exit(m.Run())
	}

	for _, addr := range append(append([]string{}, pureAddrs...), proAddrs...) {
		if waitFor(addr, 10*time.Second) {
			ftpdAddrs = append(ftpdAddrs, addr)
		}
	}

	if len(ftpdAddrs) == 0 {
		skipServers = true
		fmt.Fprintln(os.Stderr,
			"no FTP test server is listening on "+
				"2121 or 2124; server-dependent tests will be skipped.\n"+
				"Start them with: ./scripts/test-servers.sh up")
	}

	os.Exit(m.Run())
}
