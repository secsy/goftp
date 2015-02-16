// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"bytes"
	"os"
	"time"
)

func Example() {
	// Create client object with default config
	client, err := Dial("ftp.example.com")
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
	config := Config{
		User:           "jlpicard",
		Password:       "beverly123",
		MaxConnections: 20,
		Timeout:        10 * time.Second,
		Logger:         os.Stderr,
	}

	client, err := DialConfig(config, "ftp.example.com")
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
