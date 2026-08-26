package goftp

import (
	"os"
	"testing"
	"time"
)

// Microsoft's FTP Service does not produce ls-style listings. It sends a
// DOS-style one, which the unix regex cannot match at all — so ReadDir
// fails outright on any IIS server.
//
// Reported upstream three times: secsy/goftp#43, #54 and #65. Every
// entry below is copied from one of those reports.
func TestParseLISTDOSFormat(t *testing.T) {
	for _, c := range []struct {
		entry string
		name  string
		dir   bool
		size  int64
		when  time.Time
	}{
		// From #54.
		{
			entry: "10-09-20  09:36PM       <DIR>          aspnet_client",
			name:  "aspnet_client", dir: true, size: 0,
			when: time.Date(2020, 10, 9, 21, 36, 0, 0, time.UTC),
		},
		{
			entry: "10-16-20  05:20PM                 6989 Biography.html",
			name:  "Biography.html", dir: false, size: 6989,
			when: time.Date(2020, 10, 16, 17, 20, 0, 0, time.UTC),
		},
		// From #65 — a four-digit year, and a name containing a space.
		{
			entry: "03-02-2023  03:15PM       <DIR>          Archived Tracking",
			name:  "Archived Tracking", dir: true, size: 0,
			when: time.Date(2023, 3, 2, 15, 15, 0, 0, time.UTC),
		},
		// Midnight and noon are where 12-hour clocks go wrong.
		{
			entry: "01-01-21  12:00AM                    0 midnight.txt",
			name:  "midnight.txt", dir: false, size: 0,
			when: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			entry: "01-01-21  12:00PM                    1 noon.txt",
			name:  "noon.txt", dir: false, size: 1,
			when: time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	} {
		info, err := parseLIST(c.entry, time.UTC, false)
		if err != nil {
			t.Errorf("%q: %v", c.entry, err)
			continue
		}
		if info == nil {
			t.Errorf("%q: parsed to nothing", c.entry)
			continue
		}
		if info.Name() != c.name {
			t.Errorf("%q: Name() = %q, want %q", c.entry, info.Name(), c.name)
		}
		if info.IsDir() != c.dir {
			t.Errorf("%q: IsDir() = %v, want %v", c.entry, info.IsDir(), c.dir)
		}
		if info.Size() != c.size {
			t.Errorf("%q: Size() = %d, want %d", c.entry, info.Size(), c.size)
		}
		if !info.ModTime().Equal(c.when) {
			t.Errorf("%q: ModTime() = %v, want %v", c.entry, info.ModTime(), c.when)
		}
	}
}

// A directory has no size to report, and reporting one would be a
// number the server never sent.
func TestParseLISTDOSDirectoryMode(t *testing.T) {
	info, err := parseLIST("10-09-20  09:36PM       <DIR>          css", time.UTC, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeDir == 0 {
		t.Errorf("Mode() = %v, which does not say directory", info.Mode())
	}
}

// The unix format must keep working — this adds a format, it does not
// replace one.
func TestParseLISTUnixStillWorks(t *testing.T) {
	info, err := parseLIST(
		"drwxr-xr-x   8 goftp    20            272 Jul 28 05:03 git-ignored", time.UTC, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name() != "git-ignored" || !info.IsDir() {
		t.Errorf("got name=%q dir=%v, want git-ignored/true", info.Name(), info.IsDir())
	}
}

// And a line that is neither must still be rejected, rather than
// half-matched by the looser of the two patterns.
func TestParseLISTStillRejectsNonsense(t *testing.T) {
	for _, entry := range []string{
		"this is not a listing",
		"10-09-20 <DIR> missing-the-time",
		"99-99-99  99:99XX       <DIR>          bad-date",
	} {
		if info, err := parseLIST(entry, time.UTC, false); err == nil && info != nil {
			t.Errorf("parseLIST(%q) accepted it as %q", entry, info.Name())
		}
	}
}
