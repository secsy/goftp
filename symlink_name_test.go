package goftp

import (
	"os"
	"testing"
	"time"
)

// A symlink's LIST entry names the link and what it points at, separated
// by an arrow. The name of the entry is the link — that is the thing the
// directory contains, and the thing os.FileInfo.Name() means.
//
// Reported upstream as secsy/goftp#60.
func TestParseLISTSymlinkName(t *testing.T) {
	for _, c := range []struct {
		what  string
		entry string
		want  string
	}{
		{
			// The target's basename is a plausible-looking answer, which
			// is what makes this the damaging case: nothing about the
			// result says it is wrong.
			what:  "target with a path",
			entry: "lrwxrwxrwx 1 goftp goftp 18 Aug 26 12:00 config -> etc/real.conf",
			want:  "config",
		},
		{
			what:  "target in the same directory",
			entry: "lrwxrwxrwx 1 goftp goftp 9 Aug 26 12:00 link.txt -> lorem.txt",
			want:  "link.txt",
		},
		{
			what:  "absolute target",
			entry: "lrwxrwxrwx 1 goftp goftp 11 Aug 26 12:00 shortcut -> /var/log/messages",
			want:  "shortcut",
		},
		{
			// Not a symlink, so there is nothing to split and a name
			// containing an arrow must survive intact.
			what:  "a regular file whose name contains an arrow",
			entry: "-rw-r--r-- 1 goftp goftp 12 Aug 26 12:00 a -> b.txt",
			want:  "a -> b.txt",
		},
	} {
		info, err := parseLIST(c.entry, time.UTC, false)
		if err != nil {
			t.Errorf("%s: %v", c.what, err)
			continue
		}
		if info == nil {
			t.Errorf("%s: parsed to nothing", c.what)
			continue
		}
		if info.Name() != c.want {
			t.Errorf("%s: Name() = %q, want %q", c.what, info.Name(), c.want)
		}
	}
}

// The mode has to keep saying it is a link, or a caller cannot know to
// resolve it.
func TestParseLISTSymlinkMode(t *testing.T) {
	info, err := parseLIST(
		"lrwxrwxrwx 1 goftp goftp 9 Aug 26 12:00 link.txt -> lorem.txt", time.UTC, false)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Mode() = %v, which does not report a symlink", info.Mode())
	}
}
