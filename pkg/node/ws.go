package node

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/tamnd/bento/pkg/engine"
)

// wsBridgeState backs the WebSocket client global. It dials and performs the
// RFC 6455 opening handshake over a raw connection, then runs the frame protocol
// on it: the read loop turns inbound frames into message and close events, and
// writes are serialized through a per-connection queue like net does. JavaScript
// holds an integer connection id and drives send and close through host
// functions; Go drives open, message, close, and error back through the
// __bento_ws_ dispatch globals.
type wsBridgeState struct {
	netBridge
	mu     sync.Mutex
	nextID int64
	conns  map[int64]*wsConn
}

// wsConn is one live WebSocket. writes carries outbound frames to a single
// writer goroutine so they serialize; each frame is (opcode, payload).
type wsConn struct {
	id     int64
	conn   net.Conn
	writes chan wsOutFrame
	once   sync.Once
}

type wsOutFrame struct {
	opcode  byte
	payload []byte
}

// wsGUID is the RFC 6455 magic value the accept key is derived from.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func installWS(eng engine.Engine, loop LoopHost) error {
	w := &wsBridgeState{
		netBridge: netBridge{eng: eng, loop: loop},
		conns:     map[int64]*wsConn{},
	}
	for name, fn := range w.hostFuncs() {
		if err := eng.Register(name, fn); err != nil {
			return err
		}
	}
	return nil
}

func (w *wsBridgeState) hostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_ws_connect": w.connect,
		"__bento_ws_send":    w.send,
		"__bento_ws_close":   w.wsClose,
	}
}

// connect dials the target and runs the handshake on a pool goroutine. It mints
// the connection id up front and returns it so send and close can be routed
// before the open event arrives. The loop reference is taken here on the loop
// goroutine and released when the read loop ends.
func (w *wsBridgeState) connect(args []any) (any, error) {
	raw := str(args, 0)
	protocols := str(args, 1)

	w.mu.Lock()
	w.nextID++
	id := w.nextID
	w.mu.Unlock()

	w.loop.AddRef()
	w.pool(func() {
		conn, br, proto, err := w.handshake(raw, protocols)
		if err != nil {
			w.loop.Post(func() { w.loop.Unref() })
			w.emit("__bento_ws_dispatchError", id, err.Error())
			w.emit("__bento_ws_dispatchClose", id, int64(1006), "")
			return
		}
		wc := &wsConn{id: id, conn: conn, writes: make(chan wsOutFrame, 64)}
		w.mu.Lock()
		w.conns[id] = wc
		w.mu.Unlock()
		w.emit("__bento_ws_dispatchOpen", id, proto)
		w.startWriter(wc)
		w.readLoop(wc, br)
	})
	return id, nil
}

// handshake opens the transport (TLS for wss), sends the upgrade request, and
// validates the 101 response and accept key. It returns the connection and the
// buffered reader that may already hold the first frame bytes.
func (w *wsBridgeState) handshake(raw, protocols string) (net.Conn, *bufio.Reader, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid WebSocket URL: %w", err)
	}
	secure := u.Scheme == "wss"
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if secure {
			port = "443"
		} else {
			port = "80"
		}
	}
	addr := net.JoinHostPort(host, port)

	var conn net.Conn
	if secure {
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return nil, nil, "", err
	}

	var keyBytes [16]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		_ = conn.Close()
		return nil, nil, "", err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes[:])

	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	var req strings.Builder
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\n", path)
	fmt.Fprintf(&req, "Host: %s\r\n", u.Host)
	req.WriteString("Upgrade: websocket\r\n")
	req.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&req, "Sec-WebSocket-Key: %s\r\n", key)
	req.WriteString("Sec-WebSocket-Version: 13\r\n")
	if protocols != "" {
		fmt.Fprintf(&req, "Sec-WebSocket-Protocol: %s\r\n", protocols)
	}
	req.WriteString("\r\n")
	if _, err := conn.Write([]byte(req.String())); err != nil {
		_ = conn.Close()
		return nil, nil, "", err
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, nil, "", err
	}
	if !strings.Contains(statusLine, " 101 ") {
		_ = conn.Close()
		return nil, nil, "", fmt.Errorf("WebSocket handshake failed: %s", strings.TrimSpace(statusLine))
	}

	accept := ""
	proto := ""
	for {
		line, rerr := br.ReadString('\n')
		if rerr != nil {
			_ = conn.Close()
			return nil, nil, "", rerr
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		name, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "sec-websocket-accept":
			accept = strings.TrimSpace(value)
		case "sec-websocket-protocol":
			proto = strings.TrimSpace(value)
		}
	}

	if accept != acceptKey(key) {
		_ = conn.Close()
		return nil, nil, "", fmt.Errorf("WebSocket handshake failed: bad accept key")
	}
	return conn, br, proto, nil
}

// acceptKey computes the Sec-WebSocket-Accept value for a client key.
func acceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// send queues a text or binary frame. The binary flag chooses the opcode.
func (w *wsBridgeState) send(args []any) (any, error) {
	wc := w.lookup(int64(intArg(args, 0)))
	if wc == nil {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(str(args, 1))
	if err != nil {
		return nil, nil
	}
	opcode := byte(wsOpText)
	if intArg(args, 2) != 0 {
		opcode = wsOpBinary
	}
	select {
	case wc.writes <- wsOutFrame{opcode: opcode, payload: data}:
	default:
	}
	return nil, nil
}

// wsClose queues a close frame; the writer goroutine sends it and then closes
// the write side. The read loop tears down once the peer's close arrives or the
// connection ends.
func (w *wsBridgeState) wsClose(args []any) (any, error) {
	wc := w.lookup(int64(intArg(args, 0)))
	if wc == nil {
		return nil, nil
	}
	code := intArg(args, 1)
	reason := str(args, 2)
	wc.once.Do(func() {
		wc.writes <- wsOutFrame{opcode: wsOpClose, payload: closePayload(code, reason)}
		close(wc.writes)
	})
	return nil, nil
}

func (w *wsBridgeState) lookup(id int64) *wsConn {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conns[id]
}

// startWriter drains the outbound frame queue on a pool goroutine so writes
// serialize and never block the loop. On drain it closes the transport, which
// unblocks the read loop.
func (w *wsBridgeState) startWriter(wc *wsConn) {
	w.pool(func() {
		for frame := range wc.writes {
			if err := writeFrame(wc.conn, frame.opcode, frame.payload); err != nil {
				// The transport is gone; close it to unblock the read loop.
				_ = wc.conn.Close()
				return
			}
			if frame.opcode == wsOpClose {
				// After sending our close, wait for the peer's close echo in the
				// read loop rather than tearing down the transport now, so a clean
				// close carries the peer's code and reason.
				return
			}
		}
	})
}

// readLoop decodes inbound frames into events. Data frames (with continuation
// reassembly) become message events, ping frames are answered with pongs, and a
// close frame ends the loop. When the loop ends the connection is forgotten, its
// loop reference dropped, and close emitted.
func (w *wsBridgeState) readLoop(wc *wsConn, br *bufio.Reader) {
	code := 1006
	reason := ""
	var fragment []byte
	fragOpcode := byte(0)

	for {
		f, err := readFrame(br)
		if err != nil {
			break
		}
		switch f.opcode {
		case wsOpPing:
			select {
			case wc.writes <- wsOutFrame{opcode: wsOpPong, payload: f.data}:
			default:
			}
		case wsOpPong:
			// Unsolicited pongs are ignored.
		case wsOpClose:
			code, reason = parseClose(f.data)
			goto done
		case wsOpText, wsOpBinary:
			if f.fin {
				w.emitMessage(wc.id, f.opcode, f.data)
			} else {
				fragOpcode = f.opcode
				fragment = append(fragment[:0], f.data...)
			}
		case wsOpContinuation:
			fragment = append(fragment, f.data...)
			if f.fin {
				w.emitMessage(wc.id, fragOpcode, fragment)
				fragment = nil
			}
		}
	}

done:
	w.mu.Lock()
	delete(w.conns, wc.id)
	w.mu.Unlock()
	_ = wc.conn.Close()
	w.loop.Post(func() { w.loop.Unref() })
	w.emit("__bento_ws_dispatchClose", wc.id, int64(code), reason)
}

// emitMessage posts a message event. Binary frames cross as base64 with the
// binary flag so JavaScript can hand back an ArrayBuffer; text frames cross as
// the same base64 and are decoded to a string on the JS side.
func (w *wsBridgeState) emitMessage(id int64, opcode byte, data []byte) {
	binaryFlag := 0
	if opcode == wsOpBinary {
		binaryFlag = 1
	}
	w.emit("__bento_ws_dispatchMessage", id, base64.StdEncoding.EncodeToString(data), int64(binaryFlag))
}
