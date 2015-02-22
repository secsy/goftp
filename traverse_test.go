// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"
)

func mustParseTime(f, s string) time.Time {
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseMLST(t *testing.T) {
	cases := []struct {
		raw string
		exp *ftpFile
	}{
		{
			// dirs dont necessarily have size
			"modify=19991014192630;perm=fle;type=dir;unique=806U246E0B1;UNIX.group=1;UNIX.mode=0755;UNIX.owner=0; files",
			&ftpFile{
				name:  "files",
				mtime: mustParseTime(timeFormat, "19991014192630"),
				mode:  os.FileMode(0755) | os.ModeDir,
			},
		},
	}

	for _, c := range cases {
		c.exp.raw = c.raw

		got, err := parseMLST(c.raw, false)
		if err != nil {
			t.Fatal(err)
		}
		gotFile := got.(*ftpFile)
		if !reflect.DeepEqual(gotFile, c.exp) {
			t.Errorf("exp %+v\n got %+v", c.exp, gotFile)
		}
	}
}

func compareFileInfos(a, b os.FileInfo) error {
	if a.Name() != b.Name() {
		return fmt.Errorf("Name(): %s != %s", a.Name(), b.Name())
	}

	// reporting of size for directories is inconsistent
	if !a.IsDir() {
		if a.Size() != b.Size() {
			return fmt.Errorf("Size(): %d != %d", a.Size(), b.Size())
		}
	}

	if a.Mode() != b.Mode() {
		return fmt.Errorf("Mode(): %s != %s", a.Mode(), b.Mode())
	}

	if !a.ModTime().Truncate(time.Second).Equal(b.ModTime().Truncate(time.Second)) {
		return fmt.Errorf("ModTime() %s != %s", a.ModTime(), b.ModTime())
	}

	if a.IsDir() != b.IsDir() {
		return fmt.Errorf("IsDir(): %s != %s", a.IsDir(), b.IsDir())
	}

	return nil
}

func TestReadDir(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		list, err := c.ReadDir("")

		if err != nil {
			t.Fatal(err)
		}

		if len(list) != 3 {
			t.Errorf("expected 3 items, got %d", len(list))
		}

		var names []string

		for _, item := range list {
			expected, err := os.Stat("testroot/" + item.Name())
			if err != nil {
				t.Fatal(err)
			}

			if err := compareFileInfos(item, expected); err != nil {
				t.Errorf("mismatch on %s: %s (%s)", item.Name(), err, item.Sys().(string))
			}

			names = append(names, item.Name())
		}

		// sanity check names are what we expected
		sort.Strings(names)
		if !reflect.DeepEqual(names, []string{"git-ignored", "lorem.txt", "subdir"}) {
			t.Errorf("got: %v", names)
		}
	}
}

func TestNameList(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		list, err := c.NameList("")

		if err != nil {
			t.Fatal(err)
		}

		sort.Strings(list)

		if !reflect.DeepEqual([]string{"git-ignored", "lorem.txt", "subdir"}, list) {
			t.Errorf("Got %v", list)
		}

		list, err = c.NameList("subdir")

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual([]string{"1234.bin"}, list) {
			t.Errorf("Got %v", list)
		}

		if int(c.numOpenConns) != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestStat(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		// check root
		info, err := c.Stat("")
		if err != nil {
			t.Fatal(err)
		}

		// work around inconsistency between pure-ftpd and proftpd
		var realStat os.FileInfo
		if info.Name() == "testroot" {
			realStat, err = os.Stat("testroot")
		} else {
			realStat, err = os.Stat("testroot/.")
		}
		if err != nil {
			t.Fatal(err)
		}

		if err := compareFileInfos(info, realStat); err != nil {
			t.Error(err)
		}

		// check a file
		info, err = c.Stat("subdir/1234.bin")
		if err != nil {
			t.Fatal(err)
		}

		realStat, err = os.Stat("testroot/subdir/1234.bin")
		if err != nil {
			t.Fatal(err)
		}

		if err := compareFileInfos(info, realStat); err != nil {
			t.Error(err)
		}

		// check a directory
		info, err = c.Stat("subdir")
		if err != nil {
			t.Fatal(err)
		}

		realStat, err = os.Stat("testroot/subdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := compareFileInfos(info, realStat); err != nil {
			t.Error(err)
		}

		if int(c.numOpenConns) != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}
