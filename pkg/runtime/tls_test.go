package runtime

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// selfSignedPEM returns a cert and key PEM pair valid for 127.0.0.1, so a bento
// TLS server can present it and a Go client can dial it (skipping verification,
// since the cert is self-signed).
func selfSignedPEM(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<31-1, 0),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return string(certPEM), string(keyPEM)
}

func TestHTTPSServer(t *testing.T) {
	port := freePort(t)
	certPEM, keyPEM := selfSignedPEM(t)
	script := fmt.Sprintf(`
		const https = require("https");
		const options = { key: %s, cert: %s };
		const server = https.createServer(options, (req, res) => {
			res.setHeader("Content-Type", "text/plain");
			res.end("secure " + req.url);
			server.close();
		});
		server.listen(%d);
	`, jsQuote(keyPEM), jsQuote(certPEM), port)

	done, out := runServer(t, port, script)

	// Wait for the TLS port to accept, then GET over https skipping verification.
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
	}}
	waitForTLS(t, port)

	resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d/page", port))
	if err != nil {
		t.Fatalf("https get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "secure /page" {
		t.Errorf("body = %q, want %q", body, "secure /page")
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Errorf("content-type = %q", got)
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

func TestTLSEchoServerAndClient(t *testing.T) {
	port := freePort(t)
	certPEM, keyPEM := selfSignedPEM(t)
	// A bento TLS echo server and a bento TLS client in one script. The client
	// disables verification because the cert is self-signed.
	script := fmt.Sprintf(`
		const tls = require("tls");
		const server = tls.createServer({ key: %s, cert: %s }, (socket) => {
			socket.on("data", (chunk) => {
				socket.write(chunk.toString().toUpperCase());
				socket.end();
			});
		});
		server.listen(%d, () => {
			const client = tls.connect(%d, "127.0.0.1", { rejectUnauthorized: false }, () => {
				client.write("secure-ping");
			});
			let reply = "";
			client.on("data", (chunk) => { reply += chunk.toString(); });
			client.on("end", () => {
				console.log(reply);
				server.close();
			});
		});
	`, jsQuote(keyPEM), jsQuote(certPEM), port, port)

	out := strings.TrimSpace(runToEnd(t, script))
	if out != "SECURE-PING" {
		t.Errorf("tls echo = %q, want SECURE-PING", out)
	}
}

func TestHTTPSClientDelegatesToHTTP(t *testing.T) {
	// The https client wraps http.request. Pointed at a plain http URL string, it
	// must still work, proving the delegation does not mangle the request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "ok "+r.URL.Path)
	}))
	defer srv.Close()
	script := fmt.Sprintf(`
		const https = require("https");
		https.get(%s, (res) => {
			let body = "";
			res.on("data", (c) => { body += c.toString(); });
			res.on("end", () => { console.log(res.statusCode + " " + body); });
		});
	`, jsQuote(srv.URL+"/x"))
	out := strings.TrimSpace(runToEnd(t, script))
	if out != "200 ok /x" {
		t.Errorf("https delegate = %q, want %q", out, "200 ok /x")
	}
}

// waitForTLS polls until a TLS handshake against the port succeeds.
func waitForTLS(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: 100 * time.Millisecond},
			"tcp",
			net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
			&tls.Config{InsecureSkipVerify: true},
		)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tls server on port %d never came up", port)
}
