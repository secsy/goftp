package goftp

import (
	"bufio"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
)

// RoundTrip implements the http.RoundTripper interface to allow an http.Client
// to handle ftp:// or ftps:// URLs. If req.URL.User is nil, the user and password
// from config will be used instead.
// Typical usage would be to register a Config to handle
// ftp:// and/or ftps:// URLs with http.Transport.RegisterProtocol.
// The User and Password fields in Config will be used when connecting
// to the remote FTP server unless the http.Request’s URL.User is non-nil.
func (config Config) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Scheme {
	default:
		return nil, http.ErrSkipAltProtocol
	case "ftp":
	case "ftps":
		if config.TLSConfig == nil {
			config.TLSConfig = &tls.Config{}
		}
	}

	// If req.URL.User is non-nil, username and password
	// will override config even if empty.
	if req.URL.User != nil {
		config.User = req.URL.User.Username()
		config.Password, _ = req.URL.User.Password()
	}

	path := strings.TrimPrefix(req.URL.Path, "/")

	client, err := DialConfig(config, req.URL.Host)
	if err != nil {
		return nil, err
	}

	res := &http.Response{}
	switch req.Method {
	default:
		return nil, http.ErrNotSupported
	case http.MethodGet:
		// Pipe Client.Retrieve to res.Body so enable unbuffered reads
		// of large files.
		// Errors returned by Client.Retrieve (like the size check)
		// will be returned by res.Body.Read().
		r, w := io.Pipe()
		brc := &bufferedReadCloser{bufio.NewReader(r), r}
		res.Body = brc
		go func() {
			w.CloseWithError(client.Retrieve(path, w))
		}()
		_, err = brc.Peek(1) // Get error immediately on bad read

		// Simulate HTTP response semantics
		if err, ok := err.(ftpError); ok {
			res.StatusCode = err.Code()
			res.Status = err.Message()
		} else {
			res.StatusCode = 200
			res.Status = http.StatusText(res.StatusCode)
		}
	}
	return res, err
}

type bufferedReadCloser struct {
	*bufio.Reader
	rc io.ReadCloser
}

func (rc *bufferedReadCloser) Close() error {
	return rc.rc.Close()
}
