package runtime

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// freePort grabs an ephemeral port and releases it, so the bento script can bind
// a known port the test can reach. The short race between release and rebind is
// acceptable for a local test.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// runServer runs a bento script that binds a server on port, in a goroutine, and
// returns a channel that delivers the run error once the loop exits. The script
// is expected to close its server after handling so the loop drains.
func runServer(t *testing.T, port int, script string) (<-chan error, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	rt, err := New(Config{
		Argv:         []string{"bento", "server.ts", strconv.Itoa(port)},
		BentoVersion: "test",
		Stdout:       &out,
		Stderr:       &out,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		runErr := rt.RunString("server.ts", script)
		_ = rt.Close()
		done <- runErr
	}()
	return done, &out
}

// waitForServer polls until the port accepts a connection or the deadline passes.
func waitForServer(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server on port %d never came up", port)
}

// noKeepAlive is a client that closes each connection so a graceful server
// Shutdown completes promptly after the test's single request.
func noKeepAlive() *http.Client {
	return &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
}

func TestHTTPServerBasicResponse(t *testing.T) {
	port := freePort(t)
	script := fmt.Sprintf(`
		const http = require("http");
		const server = http.createServer((req, res) => {
			res.statusCode = 201;
			res.setHeader("Content-Type", "text/plain");
			res.setHeader("X-Method", req.method);
			res.end("hello " + req.url);
			server.close();
		});
		server.listen(%d);
	`, port)

	done, out := runServer(t, port, script)
	waitForServer(t, port)

	resp, err := noKeepAlive().Get(fmt.Sprintf("http://127.0.0.1:%d/world", port))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/plain" {
		t.Errorf("content-type = %q, want text/plain", got)
	}
	if got := resp.Header.Get("X-Method"); got != "GET" {
		t.Errorf("x-method = %q, want GET", got)
	}
	if string(body) != "hello /world" {
		t.Errorf("body = %q, want %q", body, "hello /world")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v (output %q)", err, out.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("loop did not exit after server.close (output %q)", out.String())
	}
}

func TestHTTPServerStreamsAndSetsMultipleCookies(t *testing.T) {
	port := freePort(t)
	script := fmt.Sprintf(`
		const http = require("http");
		const server = http.createServer((req, res) => {
			res.writeHead(200, { "Set-Cookie": ["a=1", "b=2"], "Content-Type": "text/plain" });
			res.write("chunk-one;");
			res.write("chunk-two");
			res.end();
			server.close();
		});
		server.listen(%d);
	`, port)

	done, out := runServer(t, port, script)
	waitForServer(t, port)

	resp, err := noKeepAlive().Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if string(body) != "chunk-one;chunk-two" {
		t.Errorf("body = %q, want %q", body, "chunk-one;chunk-two")
	}
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) != 2 || cookies[0] != "a=1" || cookies[1] != "b=2" {
		t.Errorf("set-cookie = %v, want [a=1 b=2]", cookies)
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

func TestHTTPServerReadsRequestBody(t *testing.T) {
	port := freePort(t)
	script := fmt.Sprintf(`
		const http = require("http");
		const server = http.createServer((req, res) => {
			let body = "";
			req.on("data", (chunk) => { body += chunk.toString(); });
			req.on("end", () => {
				res.setHeader("Content-Type", "application/json");
				res.end(JSON.stringify({ method: req.method, received: body }));
				server.close();
			});
		});
		server.listen(%d);
	`, port)

	done, out := runServer(t, port, script)
	waitForServer(t, port)

	resp, err := noKeepAlive().Post(
		fmt.Sprintf("http://127.0.0.1:%d/echo", port),
		"text/plain",
		strings.NewReader("payload-body"),
	)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	want := `{"method":"POST","received":"payload-body"}`
	if string(body) != want {
		t.Errorf("body = %q, want %q", body, want)
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
