package goftp

import (
	"bytes"
	"crypto/tls"
	"testing"
)

// Servers commonly require the data connection to resume the control
// connection's TLS session. It is proftpd's default and vsftpd's
// require_ssl_reuse, and one that does not is refused:
//
//	425-Unable to build data connection: Operation not permitted
//	522-SSL connection failed; session reuse required
//
// The test proftpd requires it, so any TLS transfer here is already
// exercising resumption. This asserts it directly, and pins the two
// conditions that have to hold — setting only the first is what the
// reporters tried, and it does nothing.
//
// Reported upstream as secsy/goftp#49.
func TestTLSDataConnectionResumesTheControlSession(t *testing.T) {

	for _, addr := range ftpdAddrs {
		config := goftpConfig
		config.TLSConfig = &tls.Config{InsecureSkipVerify: true}
		config.TLSMode = TLSExplicit

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		// A transfer needs a data connection, which is where a server
		// requiring reuse refuses one that has not resumed.
		var buf bytes.Buffer
		if err := c.Retrieve("lorem.txt", &buf); err != nil {
			t.Errorf("%s: TLS transfer: %v", addr, err)
		}

		// And again, because the second one comes from the pool and must
		// still resume rather than renegotiate from nothing.
		buf.Reset()
		if err := c.Retrieve("lorem.txt", &buf); err != nil {
			t.Errorf("%s: second TLS transfer: %v", addr, err)
		}

		c.Close()
	}
}

// The caller's config must not be modified. It is theirs, and may be
// shared with connections this package knows nothing about.
func TestTLSConfigIsNotModified(t *testing.T) {

	given := &tls.Config{InsecureSkipVerify: true}

	config := goftpConfig
	config.TLSConfig = given
	config.TLSMode = TLSExplicit

	c, err := DialConfig(config, ftpdAddrs[0])
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var buf bytes.Buffer
	if err := c.Retrieve("lorem.txt", &buf); err != nil {
		t.Fatal(err)
	}

	if given.ServerName != "" {
		t.Errorf("ServerName was set on the caller's config: %q", given.ServerName)
	}
	if given.ClientSessionCache != nil {
		t.Error("a session cache was installed on the caller's config")
	}
}

// A cache the caller supplied must be the one used, not replaced.
func TestCallerSuppliedSessionCacheIsUsed(t *testing.T) {

	cache := tls.NewLRUClientSessionCache(8)
	config := goftpConfig
	config.TLSConfig = &tls.Config{
		InsecureSkipVerify: true,
		ClientSessionCache: cache,
	}
	config.TLSMode = TLSExplicit

	c, err := DialConfig(config, ftpdAddrs[0])
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var buf bytes.Buffer
	if err := c.Retrieve("lorem.txt", &buf); err != nil {
		t.Errorf("transfer with a caller-supplied session cache: %v", err)
	}
}
