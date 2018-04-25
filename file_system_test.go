// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestDelete(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(Config{User: "goftp", Password: "rocks"}, addr)
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foo")

		err = c.Store("git-ignored/foo", bytes.NewReader([]byte{1, 2, 3, 4}))
		if err != nil {
			t.Fatal(err)
		}

		_, err = os.Open("testroot/git-ignored/foo")
		if err != nil {
			t.Fatal("file is not there?", err)
		}

		if err := c.Delete("git-ignored/foo"); err != nil {
			t.Error(err)
		}

		if err := c.Delete("git-ignored/foo"); err == nil {
			t.Error("should be some sort of errorg")
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestRename(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(Config{User: "goftp", Password: "rocks"}, addr)
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foo")

		err = c.Store("git-ignored/foo", bytes.NewReader([]byte{1, 2, 3, 4}))
		if err != nil {
			t.Fatal(err)
		}

		_, err = os.Open("testroot/git-ignored/foo")
		if err != nil {
			t.Fatal("file is not there?", err)
		}

		if err := c.Rename("git-ignored/foo", "git-ignored/bar"); err != nil {
			t.Error(err)
		}

		newContents, err := ioutil.ReadFile("testroot/git-ignored/bar")
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(newContents, []byte{1, 2, 3, 4}) {
			t.Error("file contents wrong", newContents)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestMkdirRmdir(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(Config{User: "goftp", Password: "rocks"}, addr)
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foodir")

		_, err = c.Mkdir("git-ignored/foodir")
		if err != nil {
			t.Fatal(err)
		}

		stat, err := os.Stat("testroot/git-ignored/foodir")
		if err != nil {
			t.Fatal(err)
		}

		if !stat.IsDir() {
			t.Error("should be a dir")
		}

		err = c.Rmdir("git-ignored/foodir")
		if err != nil {
			t.Fatal(err)
		}

		_, err = os.Stat("testroot/git-ignored/foodir")
		if !os.IsNotExist(err) {
			t.Error("directory should be gone")
		}

		cwd, err := c.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		os.Remove(`testroot/git-ignored/dir-with-"`)
		dir, err := c.Mkdir(`git-ignored/dir-with-"`)
		if dir != `git-ignored/dir-with-"` && dir != path.Join(cwd, `git-ignored/dir-with-"`) {
			t.Errorf("Unexpected dir-with-quote value: %s", dir)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

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
		{
			// xlightftp (windows ftp server) mlsd output I found
			"size=1089207168;type=file;modify=20090426141232; adsl TV 2009-04-22 23-55-05 Jazz Icons   Lionel Hampton Live in 1958 [Mezzo].avi",
			&ftpFile{
				name:  "adsl TV 2009-04-22 23-55-05 Jazz Icons   Lionel Hampton Live in 1958 [Mezzo].avi",
				mtime: mustParseTime(timeFormat, "20090426141232"),
				mode:  os.FileMode(0400),
				size:  1089207168,
			},
		},
		{
			// test "type=OS.unix=slink"
			"type=OS.unix=slink:;size=32;modify=20140728100902;UNIX.mode=0777;UNIX.uid=647;UNIX.gid=649;unique=fd01g1220c04; access-logs",
			&ftpFile{
				name:  "access-logs",
				mtime: mustParseTime(timeFormat, "20140728100902"),
				mode:  os.FileMode(0777) | os.ModeSymlink,
				size:  32,
			},
		},
		{
			// test "type=OS.unix=symlink"
			"modify=20150928140340;perm=adfrw;size=6;type=OS.unix=symlink;unique=801U5AA227;UNIX.group=1000;UNIX.mode=0777;UNIX.owner=1000; slinkdir",
			&ftpFile{
				name:  "slinkdir",
				mtime: mustParseTime(timeFormat, "20150928140340"),
				mode:  os.FileMode(0777) | os.ModeSymlink,
				size:  6,
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

	if !a.ModTime().Truncate(time.Minute).Equal(b.ModTime().Truncate(time.Minute)) {
		return fmt.Errorf("ModTime() %s != %s", a.ModTime(), b.ModTime())
	}

	if a.IsDir() != b.IsDir() {
		return fmt.Errorf("IsDir(): %v != %v", a.IsDir(), b.IsDir())
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

		if len(list) != 4 {
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
		if !reflect.DeepEqual(names, []string{"email%40mail.com.txt", "git-ignored", "lorem.txt", "subdir"}) {
			t.Errorf("got: %v", names)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestReadDirNoMLSD(t *testing.T) {
	// pureFTPD seems to have some issues with timestamps in LIST output
	for _, addr := range proAddrs {
		config := goftpConfig
		config.stubResponses = map[string]stubResponse{
			"MLSD ": {500, "'MLSD ': command not understood."},
		}

		c, err := DialConfig(config, addr)

		if err != nil {
			t.Fatal(err)
		}

		list, err := c.ReadDir("")

		if err != nil {
			t.Fatal(err)
		}

		if len(list) != 4 {
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
		if !reflect.DeepEqual(names, []string{"email%40mail.com.txt", "git-ignored", "lorem.txt", "subdir"}) {
			t.Errorf("got: %v", names)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
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

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}

func TestStatNoMLST(t *testing.T) {
	// pureFTPD seems to have some issues with timestamps in LIST output
	for _, addr := range proAddrs {
		config := goftpConfig
		config.stubResponses = map[string]stubResponse{
			"MLST ":                {500, "'MLST ': command not understood."},
			"MLST subdir/1234.bin": {500, "'MLST ': command not understood."},
			"MLST subdir":          {500, "'MLST ': command not understood."},
		}

		c, err := DialConfig(config, addr)

		if err != nil {
			t.Fatal(err)
		}

		// check a file
		info, err := c.Stat("subdir/1234.bin")
		if err != nil {
			t.Fatal(err)
		}

		realStat, err := os.Stat("testroot/subdir/1234.bin")
		if err != nil {
			t.Fatal(err)
		}

		if err := compareFileInfos(info, realStat); err != nil {
			t.Error(err)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}
func TestGetwd(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		cwd, err := c.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		realCwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		if cwd != "/" && cwd != path.Join(realCwd, "testroot") {
			t.Errorf("Unexpected cwd: %s", cwd)
		}

		// cd into quote directory so we can test Getwd's quote handling
		os.Remove(`testroot/git-ignored/dir-with-"`)
		dir, err := c.Mkdir(`git-ignored/dir-with-"`)
		if err != nil {
			t.Fatal(err)
		}

		pconn, err := c.getIdleConn()
		if err != nil {
			t.Fatal(err)
		}

		err = pconn.sendCommandExpected(replyFileActionOkay, "CWD %s", dir)
		c.returnConn(pconn)

		if err != nil {
			t.Fatal(err)
		}

		dir, err = c.Getwd()
		if dir != `git-ignored/dir-with-"` && dir != path.Join(cwd, `git-ignored/dir-with-"`) {
			t.Errorf("Unexpected dir-with-quote value: %s", dir)
		}

		if c.numOpenConns() != len(c.freeConnCh) {
			t.Error("Leaked a connection")
		}
	}
}
