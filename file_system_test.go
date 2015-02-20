// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

func TestDelete(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(Config{User: "goftp", Password: "rocks"}, addr)
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foo")

		err = c.Store("/git-ignored/foo", bytes.NewReader([]byte{1, 2, 3, 4}))
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
	}
}

func TestRename(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(Config{User: "goftp", Password: "rocks"}, addr)
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foo")

		err = c.Store("/git-ignored/foo", bytes.NewReader([]byte{1, 2, 3, 4}))
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
	}
}

func TestMkdirRmdir(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(Config{User: "goftp", Password: "rocks"}, addr)
		if err != nil {
			t.Fatal(err)
		}

		os.Remove("testroot/git-ignored/foodir")

		err = c.Mkdir("git-ignored/foodir")
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
	}
}
