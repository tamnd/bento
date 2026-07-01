package runtime

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHTTPUpgradeEvent drives the http server's upgrade path, which is the
// primitive the ws package rides on. A raw client sends a WebSocket handshake;
// the bento server emits upgrade, writes the 101 over the hijacked socket, and
// echoes anything the client sends after. This proves the upgrade event carries
// a real, readable and writable net.Socket without reimplementing ws.
func TestHTTPUpgradeEvent(t *testing.T) {
	port := freePort(t)
	// The RFC 6455 canonical key and its matching accept value, hardcoded so the
	// server does not need sha1 in script.
	script := fmt.Sprintf(`
		const http = require("http");
		const server = http.createServer();
		server.on("upgrade", (req, socket, head) => {
			console.log("upgrade", req.url, req.headers.upgrade, head.length);
			socket.write(
				"HTTP/1.1 101 Switching Protocols\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n"
			);
			socket.on("data", (d) => socket.write(d));
			socket.on("close", () => server.close());
		});
		server.listen(%d);
	`, port)

	done, out := runServer(t, port, script)
	waitForServer(t, port)

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	req := "GET /chat HTTP/1.1\r\n" +
		"Host: 127.0.0.1:" + strconv.Itoa(port) + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("status = %q, want 101", status)
	}
	accept := ""
	for {
		line, rerr := br.ReadString('\n')
		if rerr != nil {
			t.Fatalf("read headers: %v", rerr)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "sec-websocket-accept") {
			accept = strings.TrimSpace(value)
		}
	}
	if accept != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept = %q, want the canonical value", accept)
	}

	// The socket is a real duplex: send bytes and read them echoed back.
	if _, err := io.WriteString(conn, "framedata"); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	echo := make([]byte, len("framedata"))
	if _, err := io.ReadFull(br, echo); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(echo) != "framedata" {
		t.Fatalf("echo = %q, want framedata", echo)
	}
	_ = conn.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v (output %q)", err, out.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("loop did not drain after upgrade (output %q)", out.String())
	}
	if !strings.Contains(out.String(), "upgrade /chat websocket 0") {
		t.Fatalf("upgrade event missing or wrong: %q", out.String())
	}
}
