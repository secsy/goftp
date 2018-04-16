// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/secsy/goftp"
)

func Example() {
	// Create client object with default config
	client, err := goftp.Dial("ftp.example.com")
	if err != nil {
		panic(err)
	}

	// Download a file to disk
	readme, err := os.Create("readme")
	if err != nil {
		panic(err)
	}

	err = client.Retrieve("README", readme)
	if err != nil {
		panic(err)
	}

	// Upload a file from disk
	bigFile, err := os.Open("big_file")
	if err != nil {
		panic(err)
	}

	err = client.Store("big_file", bigFile)
	if err != nil {
		panic(err)
	}
}

func Example_config() {
	config := goftp.Config{
		User:               "jlpicard",
		Password:           "beverly123",
		ConnectionsPerHost: 10,
		Timeout:            10 * time.Second,
		Logger:             os.Stderr,
	}

	client, err := goftp.DialConfig(config, "ftp.example.com")
	if err != nil {
		panic(err)
	}

	// download to a buffer instead of file
	buf := new(bytes.Buffer)
	err = client.Retrieve("pub/interesting_file.txt", buf)
	if err != nil {
		panic(err)
	}
}

func ExampleClient_OpenRawConn() {
	// ignore errors for brevity

	client, _ := goftp.Dial("ftp.hq.nasa.gov")

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
