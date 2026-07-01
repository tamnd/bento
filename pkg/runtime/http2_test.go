package runtime

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// TestHTTP2SecureServerCompat drives the http2 compatibility server: an ordinary
// http-style handler served over HTTP/2. A Go client that only offers h2 in ALPN
// dials it, so the connection must negotiate HTTP/2, and the handler must see
// req.httpVersion === "2.0". This proves the section 4 request/response bridge is
// reused unchanged over an h2 transport.
func TestHTTP2SecureServerCompat(t *testing.T) {
	port := freePort(t)
	certPEM, keyPEM := selfSignedPEM(t)
	script := fmt.Sprintf(`
		const http2 = require("http2");
		const options = { key: %s, cert: %s };
		const server = http2.createSecureServer(options, (req, res) => {
			res.setHeader("Content-Type", "text/plain");
			res.end("h2 " + req.httpVersion + " " + req.url);
			server.close();
		});
		server.listen(%d);
	`, jsQuote(keyPEM), jsQuote(certPEM), port)

	done, out := runServer(t, port, script)

	// Offer only h2 in ALPN so the transport must use HTTP/2 or fail the handshake.
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}},
		ForceAttemptHTTP2: true,
		DisableKeepAlives: true,
	}}
	waitForTLS(t, port)

	resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d/over-h2", port))
	if err != nil {
		t.Fatalf("h2 get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.ProtoMajor != 2 {
		t.Errorf("negotiated %s, want HTTP/2", resp.Proto)
	}
	if got := string(body); got != "h2 2.0 /over-h2" {
		t.Errorf("body = %q, want %q", got, "h2 2.0 /over-h2")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("content-type = %q", ct)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v (output %q)", err, out.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("loop did not exit (output %q)", out.String())
	}
}
