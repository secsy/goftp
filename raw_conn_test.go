// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
)

func ExampleClient_OpenRawConn() {
	// ignore errors for brevity

	client, _ := Dial("ftp.hq.nasa.gov")

	rawConn, _ := client.OpenRawConn()

	code, msg, _ := rawConn.SendCommand("FEAT")
	fmt.Printf("FEAT: %d-%s\n", code, msg)

	// prepare data connection
	dcGetter, _ := rawConn.PrepareDataConn()

	// cause server to initiate data connection
	rawConn.SendCommand("LIST")

	// get actual data connection
	dc, _ := dcGetter()

	data, _ := ioutil.ReadAll(dc)
	fmt.Printf("LIST response: %s\n", data)

	// close data connection
	dc.Close()

	// read final response from server after data transfer
	code, msg, _ = rawConn.ReadResponse()
	fmt.Printf("Final response: %d-%s\n", code, msg)
}

func TestRawConn(t *testing.T) {
	for _, addr := range ftpdAddrs {
		c, err := DialConfig(goftpConfig, addr)

		if err != nil {
			t.Fatal(err)
		}

		rawConn, err := c.OpenRawConn()
		if err != nil {
			t.Fatal(err)
		}

		code, msg, err := rawConn.SendCommand("FEAT")
		if err != nil {
			t.Fatal(err)
		}

		if code != 211 {
			t.Errorf("got %d", code)
		}

		if !strings.Contains(msg, "REST") {
			t.Errorf("got %s", msg)
		}

		dcGetter, err := rawConn.PrepareDataConn()
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = rawConn.SendCommand("LIST")
		if err != nil {
			t.Fatal(err)
		}

		dc, err := dcGetter()
		if err != nil {
			t.Fatal(err)
		}

		got, err := ioutil.ReadAll(dc)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(got), "lorem.txt") {
			t.Errorf("got %s", got)
		}

		dc.Close()

		code, _, err = rawConn.ReadResponse()
		if err != nil {
			t.Fatal(err)
		}
		if code != 226 {
			t.Errorf("got: %d", code)
		}

		if err := rawConn.Close(); err != nil {
			t.Error(err)
		}
	}
}
