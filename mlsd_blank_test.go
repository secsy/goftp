package goftp

import "testing"

// Some servers end an MLSD listing with a blank line. Parsing it as an
// entry fails the whole listing, so a directory that is perfectly
// readable comes back as an error.
//
// Reported upstream as secsy/goftp#45, with a server sending:
//
//	Type=dir;Size=0;Modify=20191124122657; subdir1
//	Type=dir;Size=0;Modify=20190808091946; subdir2
//	<blank>
func TestParseMLSTBlankLine(t *testing.T) {
	for _, entry := range []string{"", " ", "\t", "\r"} {
		info, err := parseMLST(entry, true)
		if err != nil {
			t.Errorf("parseMLST(%q) returned %v; a blank line is not an entry, "+
				"and treating it as one fails the whole listing", entry, err)
		}
		if info != nil {
			t.Errorf("parseMLST(%q) produced an entry %q from nothing", entry, info.Name())
		}
	}
}

// A real entry must still parse, and a genuinely malformed one must
// still be an error — skipping blank lines must not become skipping
// anything inconvenient.
func TestParseMLSTStillParsesAndStillRejects(t *testing.T) {
	info, err := parseMLST("Type=dir;Size=0;Modify=20191124122657; subdir1", true)
	if err != nil {
		t.Fatalf("a valid entry failed: %v", err)
	}
	if info == nil || info.Name() != "subdir1" {
		t.Fatalf("got %v, want an entry named subdir1", info)
	}

	if _, err := parseMLST("this is not an MLST entry at all", true); err == nil {
		t.Error("a malformed entry was accepted")
	}
}

// Stat shares the parser with ReadDir, so making a blank line "not an
// entry" must not turn Stat into something that returns a nil FileInfo
// and a nil error. A caller checking err and then using the result would
// dereference nothing.
func TestStatRejectsAnEmptyMLSTBody(t *testing.T) {

	for _, addr := range ftpdAddrs {
		config := goftpConfig
		// A three-line MLST reply whose middle line — the entry — is
		// blank. Well-formed by shape, empty of content.
		config.stubResponses = map[string]stubResponse{
			"MLST subdir": {250, "Start\n \nEnd"},
		}

		c, err := DialConfig(config, addr)
		if err != nil {
			t.Fatal(err)
		}

		info, err := c.Stat("subdir")
		if err == nil {
			t.Errorf("%s: Stat returned no error for an empty entry (info=%v)", addr, info)
		}
		if info != nil {
			t.Errorf("%s: Stat returned an entry built from a blank line", addr)
		}
		c.Close()
	}
}
