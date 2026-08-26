package goftp

import (
	"os"
	"testing"
)

// Stat must describe a directory on a server that does not implement
// MLST, not fail on it and not describe something inside it.
//
// Reported upstream as secsy/goftp#60's sibling, secsy/goftp#26.
func TestStatDirectoryWithoutMLST(t *testing.T) {
	// proftpd, because pure-ftpd's LIST timestamps are awkward — the
	// same reason TestStatNoMLST uses it.
	for _, addr := range proAddrs {
		config := goftpConfig
		// Force the LIST fallback, the way a server without MLST would.
		config.stubResponses = map[string]stubResponse{
			"MLST subdir": {500, "'MLST': command not understood."},
		}

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatal(err)
		}

		info, err := c.Stat("subdir")
		if err != nil {
			t.Fatalf("Stat on a directory without MLST: %v", err)
		}

		if !info.IsDir() {
			t.Errorf("IsDir() = false; Stat described something else, mode %v", info.Mode())
		}
		if info.Name() != "subdir" {
			t.Errorf("Name() = %q, want %q — Stat described the wrong thing",
				info.Name(), "subdir")
		}

		realStat, err := os.Stat("testroot/subdir")
		if err != nil {
			t.Fatal(err)
		}
		if info.IsDir() != realStat.IsDir() {
			t.Errorf("IsDir() = %v, want %v", info.IsDir(), realStat.IsDir())
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
		c.Close()
	}
}

// The same, on a server that rejects ls flags.
//
// proftpd and pure-ftpd both honour "LIST -d", so the parent-scan
// fallback would never run against them — and a fallback nothing
// exercises is a fallback that does not work. IIS is the real case this
// covers; stubbing the rejection is how it gets covered here.
func TestStatDirectoryWithoutMLSTOrListDash(t *testing.T) {
	for _, addr := range proAddrs {
		config := goftpConfig
		config.stubResponses = map[string]stubResponse{
			"MLST subdir": {500, "'MLST': command not understood."},
			// What a server that does not parse ls flags says.
			"LIST -d subdir": {550, "subdir: No such file or directory"},
		}

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatal(err)
		}

		info, err := c.Stat("subdir")
		if err != nil {
			t.Fatalf("Stat fell back and still failed: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("IsDir() = false, mode %v", info.Mode())
		}
		if info.Name() != "subdir" {
			t.Errorf("Name() = %q, want \"subdir\"", info.Name())
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
		c.Close()
	}
}

// A file must still work through either route.
//
// This one passes without the fix too — LIST on a file already returns
// the file — so it proves no regression rather than proving the fix. It
// is here because the fallback changes the path a file takes as well,
// and a fix that repaired directories by breaking files would otherwise
// go unnoticed.
func TestStatFileWithoutMLSTOrListDash(t *testing.T) {
	for _, addr := range proAddrs {
		config := goftpConfig
		config.stubResponses = map[string]stubResponse{
			"MLST subdir/1234.bin":    {500, "'MLST': command not understood."},
			"LIST -d subdir/1234.bin": {550, "No such file or directory"},
		}

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatal(err)
		}

		info, err := c.Stat("subdir/1234.bin")
		if err != nil {
			t.Fatalf("Stat on a file via the parent scan: %v", err)
		}
		if info.IsDir() {
			t.Error("IsDir() = true for a regular file")
		}
		if info.Name() != "1234.bin" {
			t.Errorf("Name() = %q, want \"1234.bin\"", info.Name())
		}
		c.Close()
	}
}
