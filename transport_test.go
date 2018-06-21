package goftp

import (
	"bytes"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestTransportTimeoutConnect(t *testing.T) {
	config := Config{Timeout: 100 * time.Millisecond}
	transport := Transport{Config: config}

	req, err := http.NewRequest(http.MethodGet, "ftp://168.254.111.222:2121/subdir/1234.bin", nil)
	if err != nil {
		t.Fatal(err)
	}

	t0 := time.Now()
	res, err := transport.RoundTrip(req)
	// Transport.RoundTrip calls Client.Retrieve in a goroutine
	// so large file reads are unbuffered.
	_, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	delta := time.Now().Sub(t0)
	if err == nil || !err.(Error).Temporary() {
		t.Error("Expected a timeout error")
	}

	offBy := delta - config.Timeout
	if offBy < 0 {
		offBy = -offBy
	}
	if offBy > 50*time.Millisecond {
		t.Errorf("Timeout of 100ms was off by %s", offBy)
	}
}

func TestTransportExplicitTLS(t *testing.T) {
	for _, addr := range ftpdAddrs {
		config := Config{
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSMode: TLSExplicit,
		}
		transport := Transport{Config: config}

		req, err := http.NewRequest(http.MethodGet, "ftp://"+addr+"/subdir/1234.bin", nil)
		if err != nil {
			t.Fatal(err)
		}

		req.URL.User = url.UserPassword("goftp", "rocks")

		res, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal([]byte{1, 2, 3, 4}, b) {
			t.Errorf("Got %v", b)
		}
	}
}

func TestTransportImplicitTLS(t *testing.T) {
	closer, err := startPureFTPD(implicitTLSAddrs, "ftpd/pure-ftpd-implicittls")
	if err != nil {
		t.Fatal(err)
	}

	defer closer()

	for _, addr := range implicitTLSAddrs {
		config := Config{
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSMode: TLSImplicit,
		}
		transport := Transport{Config: config}

		req, err := http.NewRequest(http.MethodGet, "ftp://"+addr+"/subdir/1234.bin", nil)
		if err != nil {
			t.Fatal(err)
		}

		req.URL.User = url.UserPassword("goftp", "rocks")

		res, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal([]byte{1, 2, 3, 4}, b) {
			t.Errorf("Got %v", b)
		}
	}
}
