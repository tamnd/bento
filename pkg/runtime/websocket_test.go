package runtime

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// wsEchoServer is a minimal RFC 6455 server for the client test: it completes
// the handshake, then echoes each text or binary frame back and mirrors a close.
// It lives in the test rather than the runtime so the client is exercised
// against an independent implementation.
func wsEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Sec-WebSocket-Key")
		h := sha1.New()
		io.WriteString(h, key+"258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
		accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("response writer is not a Hijacker")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()

		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
		io.WriteString(buf, resp)
		buf.Flush()

		for {
			opcode, payload, err := wsReadServer(buf.Reader)
			if err != nil {
				return
			}
			switch opcode {
			case 0x8: // close: echo it and stop
				wsWriteServer(conn, 0x8, payload)
				return
			case 0x1, 0x2: // text or binary: echo back
				if err := wsWriteServer(conn, opcode, payload); err != nil {
					return
				}
			}
		}
	}))
}

// wsReadServer reads one masked client frame.
func wsReadServer(r *bufio.Reader) (byte, []byte, error) {
	b0, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	opcode := b0 & 0x0F
	b1, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	length := uint64(b1 & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	var mask [4]byte
	if _, err := io.ReadFull(r, mask[:]); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return opcode, payload, nil
}

// wsWriteServer writes one unmasked server frame.
func wsWriteServer(conn net.Conn, opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, byte(length))
	case length < 1<<16:
		header = append(header, 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(length))
		header = append(header, ext[:]...)
	default:
		header = append(header, 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		header = append(header, ext[:]...)
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func wsURL(srv *httptest.Server) string {
	return "ws://" + strings.TrimPrefix(srv.URL, "http://")
}

// TestWebSocketTextEcho opens a client, sends a text frame, and checks the echo,
// then closes cleanly so the loop drains.
func TestWebSocketTextEcho(t *testing.T) {
	srv := wsEchoServer(t)
	defer srv.Close()

	out := runToEnd(t, `
		const ws = new WebSocket(`+jsQuote(wsURL(srv))+`);
		ws.onopen = () => ws.send("hello");
		ws.onmessage = (ev) => {
			console.log("msg", ev.data, ws.readyState);
			ws.close(1000, "done");
		};
		ws.onclose = (ev) => console.log("close", ev.code, ev.wasClean);
	`)
	if !strings.Contains(out, "msg hello 1") {
		t.Fatalf("text echo failed: %q", out)
	}
	if !strings.Contains(out, "close 1000 true") {
		t.Fatalf("clean close failed: %q", out)
	}
}

// TestWebSocketBinaryEcho sends binary data and checks it arrives as an
// ArrayBuffer echoed back byte for byte.
func TestWebSocketBinaryEcho(t *testing.T) {
	srv := wsEchoServer(t)
	defer srv.Close()

	out := runToEnd(t, `
		const ws = new WebSocket(`+jsQuote(wsURL(srv))+`);
		ws.binaryType = "arraybuffer";
		ws.onopen = () => ws.send(new Uint8Array([1, 2, 3, 250]));
		ws.onmessage = (ev) => {
			const view = new Uint8Array(ev.data);
			console.log("bin", ev.data instanceof ArrayBuffer, view[0], view[3], view.length);
			ws.close();
		};
	`)
	if !strings.Contains(out, "bin true 1 250 4") {
		t.Fatalf("binary echo failed: %q", out)
	}
}

// TestWebSocketConnectError checks that a failed handshake surfaces an error and
// a close event rather than hanging the loop.
func TestWebSocketConnectError(t *testing.T) {
	// A plain HTTP server never sends 101, so the handshake fails.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer plain.Close()

	out := runToEnd(t, `
		const ws = new WebSocket(`+jsQuote(wsURL(plain))+`);
		ws.onerror = () => console.log("errored");
		ws.onclose = (ev) => console.log("closed", ev.code);
	`)
	if !strings.Contains(out, "errored") || !strings.Contains(out, "closed 1006") {
		t.Fatalf("expected error and close, got: %q", out)
	}
}
