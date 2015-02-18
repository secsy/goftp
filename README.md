# goftp - an FTP client for golang

[![Build Status](https://travis-ci.org/secsy/goftp.svg)](https://travis-ci.org/secsy/goftp) [![GoDoc](https://godoc.org/github.com/secsy/goftp?status.svg)](https://godoc.org/github.com/secsy/goftp)

API stability: wibbly-wobbly. At this point I'm still defining the API, so I'm making changes fairly willy-nilly.

goftp aims to be a high-level FTP client that takes advantage of useful FTP features when supported by the server.

Here are some notable package highlights:

* Connection pooling for parallel transfers/traversal.
* Automatic resumption of interruped file transfers.
* Explicit and implicit FTPS support (TLS only, no SSL).
* IPv6 support.
* Reasonably good automated tests that run against pure-ftpd.

Please see the godocs for details and examples.

Pull requests or feature requests are welcome, but in the case of the former, you better add tests.