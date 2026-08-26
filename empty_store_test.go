package goftp

import (
	"bytes"
	"crypto/tls"
	"testing"
)

// Storing an empty file over TLS reportedly fails with "unexpected EOF"
// while the same store without TLS succeeds.
//
// Reported upstream as secsy/goftp#63 against vsftpd.
func TestStoreEmptyFileOverTLS(t *testing.T) {

	for _, addr := range ftpdAddrs {
		config := goftpConfig
		config.TLSConfig = &tls.Config{InsecureSkipVerify: true}
		config.TLSMode = TLSExplicit

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		// Nothing at all, which is the case in the report.
		if err := c.Store("git-ignored/empty-tls.bin", bytes.NewReader(nil)); err != nil {
			t.Errorf("%s: storing an empty file over TLS: %v", addr, err)
		}

		// And it must actually be there, empty.
		var buf bytes.Buffer
		if err := c.Retrieve("git-ignored/empty-tls.bin", &buf); err != nil {
			t.Errorf("%s: reading it back: %v", addr, err)
		} else if buf.Len() != 0 {
			t.Errorf("%s: read back %d bytes, want 0", addr, buf.Len())
		}

		_ = c.Delete("git-ignored/empty-tls.bin")
		c.Close()
	}
}

// The same without TLS, which the report says works — so a failure here
// would mean the problem is not TLS at all.
func TestStoreEmptyFileWithoutTLS(t *testing.T) {

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		if err := c.Store("git-ignored/empty-plain.bin", bytes.NewReader(nil)); err != nil {
			t.Errorf("%s: storing an empty file without TLS: %v", addr, err)
		}

		_ = c.Delete("git-ignored/empty-plain.bin")
		c.Close()
	}
}
