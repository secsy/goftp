# goftp - an FTP client for golang

[![CI](https://github.com/antimatter-studios/goftp/actions/workflows/ci.yml/badge.svg)](https://github.com/antimatter-studios/goftp/actions/workflows/ci.yml) [![GoDoc](https://godoc.org/github.com/secsy/goftp?status.svg)](https://godoc.org/github.com/secsy/goftp)

goftp aims to be a high-level FTP client that takes advantage of useful FTP features when supported by the server.

Here are some notable package highlights:

* Connection pooling for parallel transfers/traversal.
* Automatic resumption of interruped file transfers.
* Explicit and implicit FTPS support (TLS only, no SSL).
* IPv6 support.
* Reasonably good automated tests that run against pure-ftpd and proftpd.

Please see the godocs for details and examples.

Pull requests or feature requests are welcome, but in the case of the former, you better add tests.

### Tests ###

The tests run against real FTP servers — pure-ftpd, with and without implicit
TLS, and proftpd — in containers. Docker is the only prerequisite.

```sh
./scripts/test-servers.sh test    # start the servers and run the suite
./scripts/test-servers.sh up      # leave them running
./scripts/test-servers.sh down    # stop them
./scripts/test-servers.sh logs    # see what they are saying
```

`test` runs the suite in a container on the servers' own network, and that is
not just tidiness. FTP negotiates transfers by address: a passive transfer has
the server tell the client where to connect, and an active one has the server
connect back. A server behind published ports names an address the host cannot
route to, so passive transfers time out and active ones are refused. Sharing a
network makes the address the server names the address the client can reach.

`up` publishes the ports as well, so `go test` from the host works for
everything that is not a data transfer.

The tests that need no server at all — parsers, helpers — run anywhere:

```sh
GOFTP_SKIP_SERVERS=1 go test ./...
```

The `go` directive in `go.mod` is this library's compatibility floor, not the
version it is developed against; CI tests both.
