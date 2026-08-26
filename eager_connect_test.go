package goftp

import (
	"net"
	"strings"
	"testing"
	"time"
)

// closedPort returns an address on loopback that nothing is listening on,
// by binding one and letting it go. Picking a number by hand risks a port
// that is filtered rather than refused, which hangs instead of failing.
func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// DialConfig is named like every other Dial in Go and behaves unlike
// them: it builds a client without connecting, so a wrong host, a
// refused port or a bad password are all reported by whatever operation
// happens to run first — or by nothing at all, if the caller only checks
// the error DialConfig returned.
//
// Reported upstream as secsy/goftp#35.
func TestEagerConnectReportsAFailureToConnect(t *testing.T) {
	addr := closedPort(t)

	c, err := DialConfig(Config{
		EagerConnect: true,
		Timeout:      5 * time.Second,
	}, addr)
	if err == nil {
		c.Close()
		t.Fatalf("DialConfig(%s) returned no error for a port nothing is listening on", addr)
	}
	if !strings.Contains(err.Error(), "refused") && !strings.Contains(err.Error(), "connect") {
		t.Errorf("error does not say what went wrong: %v", err)
	}
}

// Bad credentials are a connection failure too, and the one most likely
// to be silently ignored — the server is reachable, so nothing looks
// wrong until an operation fails much later.
func TestEagerConnectReportsBadCredentials(t *testing.T) {
	for _, addr := range ftpdAddrs {
		cfg := goftpConfig
		cfg.EagerConnect = true
		cfg.Password = "not the password"

		c, err := DialConfig(cfg, addr)
		if err == nil {
			c.Close()
			t.Errorf("%s: DialConfig returned no error for a wrong password", addr)
		}
	}
}

// The default must not change. Callers rely on DialConfig being cheap
// and on connections opening lazily.
func TestWithoutEagerConnectDialStaysLazy(t *testing.T) {
	addr := closedPort(t)

	c, err := DialConfig(Config{Timeout: 5 * time.Second}, addr)
	if err != nil {
		t.Fatalf("DialConfig became eager by default: %v", err)
	}
	defer c.Close()

	// The failure still arrives, at the first operation.
	if _, err := c.Getwd(); err == nil {
		t.Error("an operation against a closed port should fail")
	}
}

// And it must connect, not merely appear to, against a server that works.
func TestEagerConnectSucceedsAgainstARealServer(t *testing.T) {
	for _, addr := range ftpdAddrs {
		cfg := goftpConfig
		cfg.EagerConnect = true

		c, err := DialConfig(cfg, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		// The connection it opened has to be back in the pool, not
		// leaked by the check that opened it.
		if c.numOpenConns() != len(c.freeConnCh) {
			t.Errorf("%s: leaked the connection used to check connectivity", addr)
		}
		c.Close()
	}
}
