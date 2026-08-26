package goftp

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// Server-side checksums, so a caller can tell whether a remote file
// differs from a local one without downloading it.
//
// Requested upstream as secsy/goftp#17. The thread's answer at the time
// was "Stat it and compare sizes", with the acknowledgement that files
// could differ at the same size — which is exactly what a checksum is
// for.
func TestChecksumAlgorithms(t *testing.T) {

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		algos, err := c.ChecksumAlgorithms()
		if err != nil {
			t.Errorf("%s: %v", addr, err)
			c.Close()
			continue
		}
		t.Logf("%s offers %v", addr, algos)

		// Whatever it offers must be usable — an algorithm advertised
		// and then refused is worse than one never mentioned.
		for _, algo := range algos {
			if _, err := c.Checksum("lorem.txt", algo); err != nil {
				t.Errorf("%s: advertised %s but %v", addr, algo, err)
			}
		}
		c.Close()
	}
}

// The point of the feature: the server's answer must match what the
// bytes actually hash to.
func TestChecksumMatchesTheFile(t *testing.T) {

	local, err := os.ReadFile("testroot/lorem.txt")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(local)
	want := hex.EncodeToString(sum[:])

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		algos, err := c.ChecksumAlgorithms()
		if err != nil || !containsAlgorithm(algos, "SHA-256") {
			t.Logf("%s: no SHA-256, skipping", addr)
			c.Close()
			continue
		}

		got, err := c.Checksum("lorem.txt", "SHA-256")
		if err != nil {
			t.Errorf("%s: %v", addr, err)
			c.Close()
			continue
		}

		// Lowercase hex, whichever command the server answered with.
		// proftpd's HASH returns lowercase and its XSHA256 uppercase, so
		// a caller comparing against encoding/hex would fail half the
		// time if this were passed through as sent.
		if got != want {
			t.Errorf("%s: Checksum = %q, want %q", addr, got, want)
		}
		if got != strings.ToLower(got) {
			t.Errorf("%s: Checksum returned uppercase hex: %q", addr, got)
		}
		c.Close()
	}
}

// An empty algorithm asks for the best the server has, so the common
// case needs no discovery call.
func TestChecksumPicksAnAlgorithm(t *testing.T) {

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		algos, _ := c.ChecksumAlgorithms()
		got, err := c.Checksum("lorem.txt", "")

		if len(algos) == 0 {
			// A server with nothing to offer must say so plainly.
			if err == nil {
				t.Errorf("%s: returned %q from a server with no checksum support", addr, got)
			}
		} else if err != nil {
			t.Errorf("%s: %v", addr, err)
		} else if got == "" {
			t.Errorf("%s: returned an empty checksum", addr)
		}
		c.Close()
	}
}

// A file that is not there is an error, not an empty string.
func TestChecksumMissingFile(t *testing.T) {

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}
		if algos, _ := c.ChecksumAlgorithms(); len(algos) == 0 {
			c.Close()
			continue
		}
		if sum, err := c.Checksum("not-there.bin", ""); err == nil {
			t.Errorf("%s: returned %q for a file that does not exist", addr, sum)
		}
		c.Close()
	}
}

func containsAlgorithm(algos []string, want string) bool {
	for _, a := range algos {
		if strings.EqualFold(a, want) {
			return true
		}
	}
	return false
}
