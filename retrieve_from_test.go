package goftp

import (
	"bytes"
	"testing"
)

// Retrieving from an offset. Requested upstream as secsy/goftp#47.
//
// The REST plumbing already exists — Retrieve uses it to resume a failed
// transfer — so this exposes what the library can already do rather than
// teaching it something new.
func TestRetrieveFrom(t *testing.T) {

	// testroot/lorem.txt is "Lorem ipsum\n", 12 bytes.
	const whole = "Lorem ipsum\n"

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		for _, tc := range []struct {
			offset int64
			want   string
		}{
			{0, whole},
			{6, "ipsum\n"},
			{11, "\n"},
			// The whole file already read: nothing left, and not an
			// error — a caller resuming a completed transfer lands here.
			{12, ""},
		} {
			var buf bytes.Buffer
			if err := c.RetrieveFrom("lorem.txt", &buf, tc.offset); err != nil {
				t.Errorf("%s: RetrieveFrom(offset=%d): %v", addr, tc.offset, err)
				continue
			}
			if buf.String() != tc.want {
				t.Errorf("%s: RetrieveFrom(offset=%d) = %q, want %q",
					addr, tc.offset, buf.String(), tc.want)
			}
		}

		// Past the end is the caller's mistake, and saying so beats
		// returning nothing and calling it success.
		var buf bytes.Buffer
		if err := c.RetrieveFrom("lorem.txt", &buf, 13); err == nil {
			t.Errorf("%s: an offset past the end of the file was accepted", addr)
		}

		// A negative offset is meaningless and must not reach the server
		// as "REST -1".
		if err := c.RetrieveFrom("lorem.txt", &buf, -1); err == nil {
			t.Errorf("%s: a negative offset was accepted", addr)
		}

		c.Close()
	}
}

// Retrieve must keep behaving exactly as it did — it is now RetrieveFrom
// starting at zero, and that must not be observable.
func TestRetrieveUnchanged(t *testing.T) {

	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)
		if err != nil {
			t.Fatalf("%s: %v", addr, err)
		}

		var buf bytes.Buffer
		if err := c.Retrieve("lorem.txt", &buf); err != nil {
			t.Errorf("%s: %v", addr, err)
		}
		if buf.String() != "Lorem ipsum\n" {
			t.Errorf("%s: got %q", addr, buf.String())
		}
		c.Close()
	}
}
