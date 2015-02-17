// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
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
				isDir: true,
			},
		},
	}

	for _, c := range cases {
		c.exp.raw = c.raw

		got, err := parseMLST(c.raw)
		if err != nil {
			t.Fatal(err)
		}
		gotFile := got.(*ftpFile)
		if !reflect.DeepEqual(gotFile, c.exp) {
			t.Errorf("exp %+v\n got %+v", c.exp, gotFile)
		}
	}
}

func TestReadDir(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := Dial(addr)

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

			if item.Size() != expected.Size() {
				t.Errorf("%s expected %d, got %d", item.Name(), expected.Size(), item.Size())
			}

			if item.Mode() != expected.Mode() {
				t.Errorf("%s expected %s, got %s", item.Name(), expected.Mode(), item.Mode())
			}

			if !item.ModTime().Equal(expected.ModTime().Truncate(time.Second)) {
				t.Errorf("%s expected %s, got %s", item.Name(), expected.ModTime(), item.ModTime())
			}

			if item.IsDir() != expected.IsDir() {
				t.Errorf("%s expected %s, got %s", item.Name(), expected.IsDir(), item.IsDir())
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
		c, err := Dial(addr)

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
