// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

// Retrieve file "path" from server and write bytes to "dest". If the
// server supports resuming stream transfers, Retrieve will continue
// resuming a failed download as long as it continues making progress.
// Retrieve will also verify the file's size after the transfer if the
// server supports the SIZE command.
func (c *Client) Retrieve(path string, dest io.Writer) error {
	// fetch file size to check against how much we transferred
	size, err := c.size(path)
	if err != nil {
		return err
	}

	canResume := c.canResume()

	var bytesSoFar int64
	for {
		n, err := c.transferFromOffset(path, dest, nil, bytesSoFar)

		bytesSoFar += n

		if err == nil {
			break
		} else if n == 0 {
			return err
		} else if !canResume {
			return fmt.Errorf("%s (can't resume)", err)
		}
	}

	if size != -1 && bytesSoFar != size {
		return fmt.Errorf("expected %d bytes, got %d", size, bytesSoFar)
	}

	return nil
}

// Read bytes from "src" and save as file "path" on the server. If the
// server supports resuming stream transfers and "src" is an io.Seeker
// (*os.File is an io.Seeker), Store will continue resuming a failed upload
// as long as it continues making progress. Store will not attempt to
// resume an upload if the client is connected to multiple servers. Store
// will also verify the remote file's size after the transfer if the server
// supports the SIZE command.
func (c *Client) Store(path string, src io.Reader) error {

	canResume := len(c.hosts) == 1 && c.canResume()

	seeker, ok := src.(io.Seeker)
	if !ok {
		canResume = false
	}

	var (
		bytesSoFar int64
		err        error
		n          int64
	)
	for {
		if bytesSoFar > 0 {
			size, err := c.size(path)
			if err != nil {
				return err
			}
			if size == -1 {
				return fmt.Errorf("%s (resume failed)", err)
			}

			_, seekErr := seeker.Seek(size, os.SEEK_SET)
			if seekErr != nil {
				c.debug("failed seeking to %d while resuming upload to %s: %s",
					size+1,
					path,
					err,
				)
				return fmt.Errorf("%s (resume failed)", err)
			}
			bytesSoFar = size
		}

		n, err = c.transferFromOffset(path, nil, src, bytesSoFar)

		bytesSoFar += n

		if err == nil {
			break
		} else if n == 0 {
			return err
		} else if !canResume {
			return fmt.Errorf("%s (can't resume)", err)
		}
	}

	// fetch file size to check against how much we transferred
	size, err := c.size(path)
	if err != nil {
		return err
	}
	if size != -1 && size != bytesSoFar {
		return fmt.Errorf("sent %d bytes, but size is %d", bytesSoFar, size)
	}

	return nil
}

func (c *Client) transferFromOffset(path string, dest io.Writer, src io.Reader, offset int64) (int64, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return 0, err
	}

	defer c.returnConn(pconn)

	if err = pconn.setType("I"); err != nil {
		return 0, err
	}

	if offset > 0 {
		err := pconn.sendCommandExpected(replyFileActionPending, "REST %d", offset)
		if err != nil {
			return 0, err
		}
	}

	dc, err := pconn.openDataConn()
	if err != nil {
		pconn.debug("error opening data connection: %s", err)
		return 0, err
	}

	// to catch early returns
	defer dc.Close()

	var cmd string
	if dest == nil && src != nil {
		dest = dc
		cmd = "STOR"
	} else if dest != nil && src == nil {
		src = dc
		cmd = "RETR"
	} else {
		panic("this shouldn't happen")
	}

	err = pconn.sendCommandExpected(replyGroupPreliminaryReply, "%s %s", cmd, path)
	if err != nil {
		return 0, err
	}

	n, err := io.Copy(dest, src)

	if err != nil {
		pconn.broken = true
		return n, err
	}

	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}

	code, msg, err := pconn.readResponse(0)
	if err != nil {
		pconn.debug("error reading response after %s: %s", cmd, err)
		return n, err
	}

	if !positiveCompletionReply(code) {
		pconn.debug("unexpected response after %s: %d (%s)", cmd, code, msg)
		return n, fmt.Errorf("unexpected response after %s: %d (%s)", cmd, code, msg)
	}

	return n, nil
}

// Fetch SIZE of file. Returns error only on underlying connection error.
// If the server doesn't support size, it returns -1 and no error.
func (c *Client) size(path string) (int64, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return -1, err
	}

	defer c.returnConn(pconn)

	if !pconn.hasFeature("SIZE") {
		pconn.debug("server doesn't support SIZE")
		return -1, nil
	}

	code, msg, err := pconn.sendCommand("SIZE %s", path)
	if err != nil {
		return -1, err
	}

	if code != replyFileStatus {
		pconn.debug("unexpected SIZE response: %d (%s)", code, msg)
		return -1, nil
	} else {
		size, err := strconv.ParseInt(msg, 10, 64)
		if err != nil {
			pconn.debug(`failed parsing SIZE response "%s": %s`, msg, err)
			return -1, nil
		} else {
			return size, nil
		}
	}
}

func (c *Client) canResume() bool {
	pconn, err := c.getIdleConn()
	if err != nil {
		return false
	}

	defer c.returnConn(pconn)

	return pconn.hasFeatureWithArg("REST", "STREAM")
}
