// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

func (c *Client) Delete(path string) error {
	pconn, err := c.getIdleConn()
	if err != nil {
		return err
	}

	return pconn.sendCommandExpected(replyFileActionOkay, "DELE %s", path)
}

func (c *Client) Rename(from, to string) error {
	pconn, err := c.getIdleConn()
	if err != nil {
		return err
	}

	err = pconn.sendCommandExpected(replyFileActionPending, "RNFR %s", from)
	if err != nil {
		return err
	}

	return pconn.sendCommandExpected(replyFileActionOkay, "RNTO %s", to)
}

func (c *Client) Mkdir(path string) error {
	pconn, err := c.getIdleConn()
	if err != nil {
		return err
	}

	return pconn.sendCommandExpected(replyDirCreated, "MKD %s", path)
}

func (c *Client) Rmdir(path string) error {
	pconn, err := c.getIdleConn()
	if err != nil {
		return err
	}

	return pconn.sendCommandExpected(replyFileActionOkay, "RMD %s", path)
}
