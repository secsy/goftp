// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const timeFormat = "20060102150405"

// Fetch the contents of a directory, returning a list of os.FileInfo's which
// are relatively easy to work with programatically. It will not return
// entries corresponding to the current directory or parent directories. This
// function will only work with servers that support the "MLSD" feature.
// FileInfo.Sys() will return the raw info string for the entry. If the server
// does not provide the "UNIX.mode" fact, the Mode() will only have UNIX bits
// set for "user" (i.e. nothing set for "group" or "user").
func (c *Client) ReadDir(path string) ([]os.FileInfo, error) {
	entries, err := c.stringList("MLSD", path)
	if err != nil {
		return nil, err
	}

	var ret []os.FileInfo
	for _, entry := range entries {
		info, err := parseMLST(entry)
		if err != nil {
			c.debug("error in ReadDir: %s", err)
			return nil, err
		}

		if info == nil {
			continue
		}

		ret = append(ret, info)
	}

	return ret, nil
}

// Retrieve a listing of file names in directory "path".
func (c *Client) NameList(path string) ([]string, error) {
	return c.stringList("NLST", path)
}

func (c *Client) stringList(cmd, path string) ([]string, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return nil, err
	}

	defer c.returnConn(pconn)

	dc, err := pconn.openDataConn()
	if err != nil {
		pconn.debug("error opening data connection: %s", err)
		return nil, err
	}

	// to catch early returns
	defer dc.Close()

	err = pconn.sendCommandExpected(replyGroupPreliminaryReply, "%s %s", cmd, path)

	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(dc)
	scanner.Split(bufio.ScanLines)

	var res []string
	for scanner.Scan() {
		res = append(res, scanner.Text())
	}

	var dataError error
	if err = scanner.Err(); err != nil {
		pconn.debug("error reading %s data: %s", cmd, err)
		dataError = fmt.Errorf("error reading %s data: %s", cmd, err)
	}

	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}

	code, msg, err := pconn.readResponse(0)
	if err != nil {
		pconn.debug("error reading response: %s", err)
		return nil, err
	}

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected result: %d-%s", code, msg)
		return nil, fmt.Errorf("unexpected result: %d (%s)", code, msg)
	}

	if dataError != nil {
		return nil, dataError
	}

	return res, nil
}

type ftpFile struct {
	name  string
	size  int64
	mode  os.FileMode
	mtime time.Time
	isDir bool
	raw   string
}

func (f *ftpFile) Name() string {
	return f.name
}

func (f *ftpFile) Size() int64 {
	return f.size
}

func (f *ftpFile) Mode() os.FileMode {
	return f.mode
}

func (f *ftpFile) ModTime() time.Time {
	return f.mtime
}

func (f *ftpFile) IsDir() bool {
	return f.isDir
}

func (f *ftpFile) Sys() interface{} {
	return f.raw
}

// an entry looks something like this:
// type=file;size=12;modify=20150216084148;UNIX.mode=0644;unique=1000004g1187ec7; lorem.txt
func parseMLST(entry string) (os.FileInfo, error) {
	parseError := fmt.Errorf(`failed parsing MLSD entry: %s`, entry)
	incompleteError := fmt.Errorf(`MLSD entry incomplete: %s`, entry)

	parts := strings.Split(entry, "; ")
	if len(parts) != 2 {
		return nil, parseError
	}

	facts := make(map[string]string)
	for _, factPair := range strings.Split(parts[0], ";") {
		factParts := strings.Split(factPair, "=")
		if len(factParts) != 2 {
			return nil, parseError
		}
		facts[strings.ToLower(factParts[0])] = strings.ToLower(factParts[1])
	}

	if facts["type"] == "" {
		return nil, incompleteError
	}

	if facts["type"] == "cdir" || facts["type"] == "pdir" || parts[1] == "." || parts[1] == ".." {
		return nil, nil
	}

	var (
		size int64
		err  error
	)
	if facts["size"] != "" {
		size, err = strconv.ParseInt(facts["size"], 10, 64)
	} else if facts["type"] == "dir" && facts["sizd"] != "" {
		size, err = strconv.ParseInt(facts["sizd"], 10, 64)
	} else if facts["type"] == "file" {
		return nil, incompleteError
	}

	if facts["modify"] == "" {
		return nil, incompleteError
	}

	mtime, err := time.ParseInLocation(timeFormat, facts["modify"], time.UTC)
	if err != nil {
		return nil, incompleteError
	}

	var mode os.FileMode
	if facts["unix.mode"] != "" {
		m, err := strconv.ParseInt(facts["unix.mode"], 8, 32)
		if err != nil {
			return nil, parseError
		}
		mode = os.FileMode(m)
	} else if facts["perm"] != "" {
		// see http://tools.ietf.org/html/rfc3659#section-7.5.5
		for _, c := range facts["perm"] {
			switch c {
			case 'a', 'd', 'c', 'f', 'm', 'p', 'w':
				// these suggest you have write permissions
				mode |= 0200
			case 'l':
				// can list dir entries means readable and executable
				mode |= 0500
			case 'r':
				// readable file
				mode |= 0400
			}
		}
	} else {
		return nil, incompleteError
	}

	if facts["type"] == "dir" {
		mode |= os.ModeDir
	}

	info := &ftpFile{
		name:  parts[1],
		size:  size,
		mtime: mtime,
		isDir: facts["type"] == "dir",
		raw:   entry,
		mode:  mode,
	}

	return info, nil
}
