package goftp

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// A connection that is not a socket, to prove the package uses whatever
// DialFunc hands back rather than dialling for itself.
type fakeConn struct{ net.Conn }

var errRefused = errors.New("dial refused by the test")

// Every connection the package opens goes through Config.dial, so a
// DialFunc reaching it here reaches the data connections too. That is
// the property worth pinning: a caller supplying one to count bytes
// would otherwise silently miss every transfer, which is most of the
// bytes there are.
func TestConfigDialUsesDialFunc(t *testing.T) {
	var got []string
	cfg := Config{
		DialFunc: func(network, address string) (net.Conn, error) {
			got = append(got, network+" "+address)
			return fakeConn{}, nil
		},
	}

	conn, err := cfg.dial("tcp", "example.invalid:21")
	if err != nil {
		t.Fatalf("dial returned %v, want nil", err)
	}
	if _, ok := conn.(fakeConn); !ok {
		t.Errorf("dial returned %T, want the connection DialFunc supplied", conn)
	}
	if len(got) != 1 || got[0] != "tcp example.invalid:21" {
		t.Errorf("DialFunc saw %v, want one call for tcp example.invalid:21", got)
	}
}

// An error from DialFunc is the caller's to see, unchanged. Wrapping it
// would hide the reason a proxy or tunnel refused.
func TestConfigDialPropagatesDialFuncError(t *testing.T) {
	cfg := Config{
		DialFunc: func(string, string) (net.Conn, error) { return nil, errRefused },
	}
	if _, err := cfg.dial("tcp", "example.invalid:21"); !errors.Is(err, errRefused) {
		t.Errorf("dial returned %v, want %v", err, errRefused)
	}
}

// Without one, nothing changes: the package dials for itself and applies
// Config.Timeout, which DialFunc callers take over.
func TestConfigDialWithoutDialFuncStillDials(t *testing.T) {
	// Port 0 on the loopback address is never listening, so this
	// exercises the built-in path and fails fast rather than hanging.
	cfg := Config{Timeout: 100 * time.Millisecond}
	if _, err := cfg.dial("tcp", "127.0.0.1:0"); err == nil {
		t.Error("dialling a port nothing listens on should fail")
	}
}

// The client must reach the hook rather than dialling around it.
func TestClientControlConnectionUsesDialFunc(t *testing.T) {
	called := 0
	client, err := DialConfig(Config{
		DialFunc: func(string, string) (net.Conn, error) {
			called++
			return nil, errRefused
		},
	}, "127.0.0.1:21")
	if err != nil {
		t.Fatalf("DialConfig returned %v", err)
	}
	defer client.Close()

	// Any operation opens the control connection.
	//
	// The package wraps the failure in its own error type, which does
	// not implement Unwrap, so the assertion is on the message it
	// carries rather than on errors.Is. That the reason survives at all
	// is what matters here: a caller whose proxy refused needs to be
	// told why.
	_, err = client.Getwd()
	if err == nil {
		t.Fatal("Getwd succeeded against a DialFunc that refuses")
	}
	if !strings.Contains(err.Error(), errRefused.Error()) {
		t.Errorf("Getwd returned %q, which does not carry %q", err, errRefused)
	}
	if called == 0 {
		t.Error("the client dialled without going through DialFunc")
	}
}
