# goftp - an FTP client for golang

[![Build Status](https://travis-ci.org/secsy/goftp.svg)](https://travis-ci.org/secsy/goftp) [![GoDoc](https://godoc.org/github.com/secsy/goftp?status.svg)](https://godoc.org/github.com/secsy/goftp)

API stability: wibbly-wobbly. At this point I'm still defining the API, so I'm making changes fairly willy-nilly.

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

How to run tests (windows not supported):
* ```./build_test_server.sh``` from root goftp directory (this downloads and compiles pure-ftpd and proftpd)
* ```go test``` from the root goftp directory

### Other FTP client features/ideas ###

If you want to contribute, you can pick something you like from this list:
* Data connection assurance (ESTA/ESTP). This FTP addition tries to fix a security hole where an attacker can hijack the data connection of an authenticated user and send/receieve files as that user. Pure-ftpd supports it, but I decided not to implement it because TLS (FTPS) solves this and all other security issues with FTP in a comprehensive way. Plus ESTA/ESTP can't guarantee to fix the problem since if an attacker is able to guess or figure out the active data connection port, she can still perpetrate the attack.
* Segmented (parallel) download. With this feature, when retrieving a file you first fetch its size, then chunk it up into, say, ten pieces (perhaps only if it is bigger than a certain threshold, say, 10MB). Then you proceed to fetch each chunk independently using the "REST" command to start from an arbitrary offset. You don't want to stream to memory since the file might be big, and you might
finish the chunks out of order, so I was going to stream to temporary files on disk as needed, then put them back together.
* FTP keep alives. I considered sending an FTP "NOOP" command every so often on idle connections to keep them active. This is not very good client manners, though. Perhaps instead goftp should track when a connection is put back in the free connection pool, and when getting a connection out of the pool if it's been idle for a certain period, check to make sure the connection is still active before trying to use it and potentially returning an error to the user.
* Use MDTM in addition to SIZE to verify downloads. After finishing a download (especially if we were interrupted and had to resume) it might be a good idea to also check the remote file's modify time before and after to make sure the file wasn't updated on the remote server.
* Add support for FTP "account". This is another credential used along with user and password for login. I'm not sure if it is actually used anywhere, so I haven't added it.
* Add data connection timeouts. Not sure how important this is since I imagine most servers time stalled transfers out eventually.