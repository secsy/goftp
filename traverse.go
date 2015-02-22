// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// time.Parse format string for parsing file mtimes.
const timeFormat = "20060102150405"

// ReadDir fetches the contents of a directory, returning a list of
// os.FileInfo's which are relatively easy to work with programatically. It
// will not return entries corresponding to the current directory or parent
// directories. ReadDir only works with servers that support the "MLST" feature.
// FileInfo.Sys() will return the raw info string for the entry. If the server
// does not provide the "UNIX.mode" fact, the Mode() will only have UNIX bits
// set for "user" (i.e. nothing set for "group" or "world").
func (c *Client) ReadDir(path string) ([]os.FileInfo, error) {
	entries, err := c.dataStringList("MLSD %s", path)
	if err != nil {
		return nil, err
	}

	var ret []os.FileInfo
	for _, entry := range entries {
		info, err := parseMLST(entry, true)
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

// Stat fetches details for a particular file. Stat requires the server to
// support the "MLST" feature.  If the server does not provide the "UNIX.mode"
// fact, the Mode() will only have UNIX bits set for "user" (i.e. nothing set
// for "group" or "world").
func (c *Client) Stat(path string) (os.FileInfo, error) {
	lines, err := c.controlStringList("MLST %s", path)
	if err != nil {
		return nil, err
	}

	if len(lines) != 3 {
		return nil, ftpError{err: fmt.Errorf("unexpected MLST response: %v", lines)}
	}

	return parseMLST(strings.TrimLeft(lines[1], " "), false)
}

// NameList fetches the contents of directory "path". If supported, ReadDir
// should be preferred over NameList.
func (c *Client) NameList(path string) ([]string, error) {
	names, err := c.dataStringList("NLST %s", path)
	if err != nil {
		return nil, err
	}

	for i := range names {
		names[i] = filepath.Base(names[i])
	}

	return names, nil
}

func (c *Client) controlStringList(f string, args ...interface{}) ([]string, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return nil, err
	}

	defer c.returnConn(pconn)

	cmd := fmt.Sprintf(f, args...)

	code, msg, err := pconn.sendCommand(cmd)

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected response to %s: %d-%s", cmd, code, msg)
		return nil, ftpError{code: code, msg: msg}
	}

	return strings.Split(msg, "\n"), nil
}

func (c *Client) dataStringList(f string, args ...interface{}) ([]string, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return nil, err
	}

	defer c.returnConn(pconn)

	dc, err := pconn.openDataConn()
	if err != nil {
		return nil, err
	}

	// to catch early returns
	defer dc.Close()

	cmd := fmt.Sprintf(f, args...)

	err = pconn.sendCommandExpected(replyGroupPreliminaryReply, cmd)

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
		dataError = ftpError{
			err:       fmt.Errorf("error reading %s data: %s", cmd, err),
			temporary: true,
		}
	}

	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}

	code, msg, err := pconn.readResponse()
	if err != nil {
		return nil, err
	}

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected result: %d-%s", code, msg)
		return nil, ftpError{code: code, msg: msg}
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
	return f.mode.IsDir()
}

func (f *ftpFile) Sys() interface{} {
	return f.raw
}

// an entry looks something like this:
// type=file;size=12;modify=20150216084148;UNIX.mode=0644;unique=1000004g1187ec7; lorem.txt
func parseMLST(entry string, skipSelfParent bool) (os.FileInfo, error) {
	parseError := ftpError{err: fmt.Errorf(`failed parsing MLST entry: %s`, entry)}
	incompleteError := ftpError{err: fmt.Errorf(`MLST entry incomplete: %s`, entry)}

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

	typ := facts["type"]

	if typ == "" {
		return nil, incompleteError
	}

	if skipSelfParent && (typ == "cdir" || typ == "pdir" || typ == "." || typ == "..") {
		return nil, nil
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

	if typ == "dir" || typ == "cdir" || typ == "pdir" {
		mode |= os.ModeDir
	}

	var (
		size int64
		err  error
	)

	if facts["size"] != "" {
		size, err = strconv.ParseInt(facts["size"], 10, 64)
	} else if mode.IsDir() && facts["sizd"] != "" {
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

	info := &ftpFile{
		name:  filepath.Base(parts[1]),
		size:  size,
		mtime: mtime,
		raw:   entry,
		mode:  mode,
	}

	return info, nil
}
