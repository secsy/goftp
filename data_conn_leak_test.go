package goftp

import (
	"bytes"
	"os"
	"runtime"
	"testing"
)

// openFDs counts this process's open file descriptors.
//
// Only meaningful on Linux, which is where the suite runs in CI and in
// the containers; elsewhere the test skips rather than guessing.
func openFDs(t *testing.T) int {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("counting open descriptors is Linux-only; this is %s", runtime.GOOS)
	}
	names, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("cannot read /proc/self/fd: %v", err)
	}
	return len(names)
}

// A transfer command that fails must not leave the data connection open.
//
// prepareDataConn opens the connection before the command is sent. If the
// command is then refused — a file that is not there, a directory that
// cannot be written — the early return skipped the close, because the
// deferred close is only set up after the getter has run.
//
// Reported upstream as secsy/goftp#46.
func TestFailedTransferDoesNotLeakTheDataConnection(t *testing.T) {

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		// One failure first, so any one-off allocation the connection
		// pool makes is already accounted for and not counted as a leak.
		var buf bytes.Buffer
		_ = c.Retrieve("does-not-exist.bin", &buf)

		before := openFDs(t)

		const attempts = 25
		for i := 0; i < attempts; i++ {
			if err := c.Retrieve("does-not-exist.bin", &buf); err == nil {
				t.Fatal("retrieving a file that is not there should fail")
			}
		}

		after := openFDs(t)

		// A leak is one descriptor per attempt. Allow a couple for the
		// pool opening a fresh control connection along the way.
		if after-before > 3 {
			t.Errorf("%s: %d descriptors open before %d failed transfers, %d after — "+
				"a leak of %d", addr, before, attempts, after, after-before)
		}
		c.Close()
	}
}
