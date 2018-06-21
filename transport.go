package goftp

import (
	"crypto/tls"
	"io"
	"net/http"
	"strings"
)

// Transport implements the http.RoundTripper interface.
// Typical usage would be to register a Transport to handle
// ftp:// and/or ftps:// URLs with http.Transport.RegisterProtocol.
type Transport struct {
	Config
}

// RoundTrip implements the http.RoundTripper interface to allow an http.Client
// to handle ftp:// or ftps:// URLs.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	config := t.Config
	switch req.URL.Scheme {
	default:
		return nil, http.ErrSkipAltProtocol
	case "ftp":
	case "ftps":
		if config.TLSConfig == nil {
			config.TLSConfig = &tls.Config{}
		}
	}

	if req.URL.User != nil {
		if user := req.URL.User.Username(); user != "" {
			config.User = user
		}
		if password, ok := req.URL.User.Password(); ok {
			config.Password = password
		}
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
		res.Body = r
		go func() {
			w.CloseWithError(client.Retrieve(path, w))
		}()
	}
	return res, err
}
