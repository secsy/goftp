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

func TestRoundTripperSkipAltProtocol(t *testing.T) {
	config := Config{}

	req, err := http.NewRequest(http.MethodGet, "foo://localhost/foo.txt", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.RoundTrip(req)
	if err != http.ErrSkipAltProtocol {
		t.Errorf("Expected err = %v, got %v", http.ErrSkipAltProtocol, err)
	}
}

func TestRoundTripperTimeoutConnect(t *testing.T) {
	config := Config{Timeout: 100 * time.Millisecond}

	req, err := http.NewRequest(http.MethodGet, "ftp://168.254.111.222:2121/subdir/1234.bin", nil)
	if err != nil {
		t.Fatal(err)
	}

	t0 := time.Now()
	res, _ := config.RoundTrip(req)
	// Config.RoundTrip calls Client.Retrieve in a goroutine
	// so large file reads are unbuffered.
	_, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	delta := time.Since(t0)
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

func TestRoundTripperExplicitTLS(t *testing.T) {
	for _, addr := range ftpdAddrs {
		config := Config{
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSMode: TLSExplicit,
		}

		req, err := http.NewRequest(http.MethodGet, "ftp://"+addr+"/subdir/1234.bin", nil)
		if err != nil {
			t.Fatal(err)
		}

		req.URL.User = url.UserPassword("goftp", "rocks")

		res, err := config.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}

		if want, got := http.StatusOK, res.StatusCode; want != got {
			t.Errorf("res.StatusCode: want: %v got: %v", want, got)
		}
		if want, got := http.StatusText(http.StatusOK), res.Status; want != got {
			t.Errorf("res.Status: want: %v got: %v", want, got)
		}

		b, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal([]byte{1, 2, 3, 4}, b) {
			t.Errorf("Got %v", b)
		}

		// Test nonexistent file
		req, err = http.NewRequest(http.MethodGet, "ftp://"+addr+"/nonexistent.file", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.URL.User = url.UserPassword("goftp", "rocks")
		res, err = config.RoundTrip(req)
		if err == nil {
			t.Errorf("expected non-nil err")
		}
		if want, got := replyFileError, res.StatusCode; want != got {
			t.Errorf("res.StatusCode: want: %v got: %v", want, got)
		}
	}
}

func TestRoundTripperImplicitTLS(t *testing.T) {
	for _, addr := range implicitTLSAddrs {
		config := Config{
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			TLSMode: TLSImplicit,
		}

		req, err := http.NewRequest(http.MethodGet, "ftp://"+addr+"/subdir/1234.bin", nil)
		if err != nil {
			t.Fatal(err)
		}

		req.URL.User = url.UserPassword("goftp", "rocks")

		res, err := config.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}

		if want, got := http.StatusOK, res.StatusCode; want != got {
			t.Errorf("res.StatusCode: want: %v got: %v", want, got)
		}
		if want, got := http.StatusText(http.StatusOK), res.Status; want != got {
			t.Errorf("res.Status: want: %v got: %v", want, got)
		}

		b, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal([]byte{1, 2, 3, 4}, b) {
			t.Errorf("Got %v", b)
		}

		// Test nonexistent file
		req, err = http.NewRequest(http.MethodGet, "ftp://"+addr+"/nonexistent.file", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.URL.User = url.UserPassword("goftp", "rocks")
		res, err = config.RoundTrip(req)
		if err == nil {
			t.Errorf("expected non-nil err")
		}
		if want, got := replyFileError, res.StatusCode; want != got {
			t.Errorf("res.StatusCode: want: %v got: %v", want, got)
		}
	}
}
