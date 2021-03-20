// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync/atomic"

	"github.com/secsy/goftp"
)

// Just for fun, walk an ftp server in parallel. I make no claim that this is
// correct or a good idea.
func ExampleClient_ReadDir_parallelWalk() {
	client, err := goftp.Dial("ftp.hq.nasa.gov")
	if err != nil {
		panic(err)
	}

	Walk(client, "", func(fullPath string, info os.FileInfo, err error) error {
		if err != nil {
			// no permissions is okay, keep walking
			if err.(goftp.Error).Code() == 550 {
				return nil
			}
			return err
		}

		fmt.Println(fullPath)

		return nil
	})
}

// Walk a FTP file tree in parallel with prunability and error handling.
// See http://golang.org/pkg/path/filepath/#Walk for interface details.
func Walk(client *goftp.Client, root string, walkFn filepath.WalkFunc) (ret error) {
	dirsToCheck := make(chan string, 100)

	var workCount int32 = 1
	dirsToCheck <- root

	for dir := range dirsToCheck {
		go func(dir string) {
			files, err := client.ReadDir(dir)

			if err != nil {
				if err = walkFn(dir, nil, err); err != nil && err != filepath.SkipDir {
					ret = err
					close(dirsToCheck)
					return
				}
			}

			for _, file := range files {
				if err = walkFn(path.Join(dir, file.Name()), file, nil); err != nil {
					if file.IsDir() && err == filepath.SkipDir {
						continue
					}
					ret = err
					close(dirsToCheck)
					return
				}

				if file.IsDir() {
					atomic.AddInt32(&workCount, 1)
					dirsToCheck <- path.Join(dir, file.Name())
				}
			}

			atomic.AddInt32(&workCount, -1)
			if workCount == 0 {
				close(dirsToCheck)
			}
		}(dir)
	}

	return ret
}
