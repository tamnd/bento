package node

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/net/http2"

	"github.com/tamnd/bento/pkg/engine"
)

// http2Bridge backs the http2 core client: http2.connect and session.request.
// Where the compatibility server reuses net/http, the client wants explicit
// control of one multiplexed connection, so it dials TLS with h2 over ALPN and
// holds a golang.org/x/net/http2 ClientConn. That ClientConn is the session; each
// session.request runs a RoundTrip on it, which opens one HTTP/2 stream over the
// shared connection, exactly the multiplexing Node's client exposes.
//
// The request body is buffered and handed over whole on end, matching the http
// client; a streaming request body over an io.Pipe is a later refinement. Response
// bodies stream back chunk by chunk through the dispatch globals, keyed by the
// stream id JavaScript minted.
type http2Bridge struct {
	netBridge
	mu       sync.Mutex
	sessions map[int64]*http2Session
}

// http2Session is one client connection. cc is the multiplexed HTTP/2 connection
// and conn is the TLS socket under it, kept so close tears both down.
type http2Session struct {
	id   int64
	cc   *http2.ClientConn
	conn net.Conn
}

// installHTTP2Client registers the http2 client host functions. The js/http2.js
// factory supplies the JavaScript half; this only wires the Go side.
func installHTTP2Client(eng engine.Engine, loop LoopHost) error {
	b := &http2Bridge{
		netBridge: netBridge{eng: eng, loop: loop},
		sessions:  map[int64]*http2Session{},
	}
	funcs := map[string]HostFunc{
		"__bento_http2_connect":      b.connect,
		"__bento_http2_request":      b.request,
		"__bento_http2_closeSession": b.closeSession,
	}
	for name, fn := range funcs {
		if err := eng.Register(name, fn); err != nil {
			return err
		}
	}
	return nil
}

// connectOptions is the subset of http2.connect options the client honors. TLS
// verification can be turned off for self-signed peers, matching Node's
// rejectUnauthorized.
type connectOptions struct {
	RejectUnauthorized *bool `json:"rejectUnauthorized"`
}

// connect dials the authority over TLS with h2 in ALPN and builds the client
// connection. It holds a loop reference for the life of the session, released by
// closeSession, so an open client keeps the runtime alive like an open server.
func (b *http2Bridge) connect(args []any) (any, error) {
	id := int64(intArg(args, 0))
	authority := str(args, 1)
	var opts connectOptions
	if raw := str(args, 2); raw != "" {
		_ = json.Unmarshal([]byte(raw), &opts)
	}

	u, err := url.Parse(authority)
	if err != nil {
		b.emit("__bento_http2_dispatchSessionError", id, err.Error())
		return nil, nil
	}
	host := u.Host
	if u.Port() == "" {
		port := "443"
		if u.Scheme == "http" {
			port = "80"
		}
		host = net.JoinHostPort(u.Hostname(), port)
	}
	insecure := opts.RejectUnauthorized != nil && !*opts.RejectUnauthorized

	b.loop.AddRef()
	b.pool(func() {
		cfg := &tls.Config{
			ServerName:         u.Hostname(),
			NextProtos:         []string{"h2"},
			InsecureSkipVerify: insecure,
		}
		conn, err := tls.Dial("tcp", host, cfg)
		if err != nil {
			b.emit("__bento_http2_dispatchSessionError", id, err.Error())
			b.loop.Post(func() { b.loop.Unref() })
			return
		}
		tr := &http2.Transport{}
		cc, err := tr.NewClientConn(conn)
		if err != nil {
			_ = conn.Close()
			b.emit("__bento_http2_dispatchSessionError", id, err.Error())
			b.loop.Post(func() { b.loop.Unref() })
			return
		}
		b.mu.Lock()
		b.sessions[id] = &http2Session{id: id, cc: cc, conn: conn}
		b.mu.Unlock()
		b.emit("__bento_http2_dispatchConnect", id)
	})
	return nil, nil
}

// request opens one stream on the session. The headers carry HTTP/2 pseudo-headers
// (:method, :path, :scheme, :authority) alongside ordinary headers, so the request
// is rebuilt from them. Each request takes its own loop reference for the duration
// of the round trip, released when the stream settles.
func (b *http2Bridge) request(args []any) (any, error) {
	sessionID := int64(intArg(args, 0))
	streamID := int64(intArg(args, 1))
	pairs := decodeHeaderPairs(str(args, 2))
	var body []byte
	if enc := str(args, 3); enc != "" {
		if dec, decErr := base64.StdEncoding.DecodeString(enc); decErr == nil {
			body = dec
		}
	}

	b.mu.Lock()
	sess := b.sessions[sessionID]
	b.mu.Unlock()
	if sess == nil {
		b.emit("__bento_http2_dispatchStreamError", streamID, "session is closed")
		return nil, nil
	}

	method, scheme, authority, path, headers := splitPseudoHeaders(pairs)
	if method == "" {
		method = http.MethodGet
	}
	if scheme == "" {
		scheme = "https"
	}
	if path == "" {
		path = "/"
	}
	target := scheme + "://" + authority + path

	b.loop.AddRef()
	b.pool(func() {
		defer b.loop.Post(func() { b.loop.Unref() })

		var reader io.Reader
		if len(body) > 0 {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, target, reader)
		if err != nil {
			b.emit("__bento_http2_dispatchStreamError", streamID, err.Error())
			return
		}
		for _, p := range headers {
			req.Header.Add(p[0], p[1])
		}

		resp, err := sess.cc.RoundTrip(req)
		if err != nil {
			b.emit("__bento_http2_dispatchStreamError", streamID, err.Error())
			return
		}
		b.emit("__bento_http2_dispatchResponse", streamID, buildResponseInfo(resp))

		buf := make([]byte, 64*1024)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				b.emit("__bento_http2_dispatchData", streamID, base64.StdEncoding.EncodeToString(buf[:n]))
			}
			if rerr != nil {
				_ = resp.Body.Close()
				if rerr == io.EOF {
					b.emit("__bento_http2_dispatchEnd", streamID)
				} else {
					b.emit("__bento_http2_dispatchStreamError", streamID, rerr.Error())
				}
				return
			}
		}
	})
	return nil, nil
}

// splitPseudoHeaders separates the HTTP/2 request pseudo-headers from the ordinary
// ones. A leading colon marks a pseudo-header; the four request ones drive the
// request line and the rest pass through as normal headers.
func splitPseudoHeaders(pairs [][2]string) (method, scheme, authority, path string, headers [][2]string) {
	for _, p := range pairs {
		switch p[0] {
		case ":method":
			method = p[1]
		case ":scheme":
			scheme = p[1]
		case ":authority":
			authority = p[1]
		case ":path":
			path = p[1]
		default:
			if len(p[0]) > 0 && p[0][0] == ':' {
				continue
			}
			headers = append(headers, p)
		}
	}
	return method, scheme, authority, path, headers
}

// closeSession tears down a session and its connection and drops the loop
// reference connect took, letting the runtime exit once nothing else is pending.
func (b *http2Bridge) closeSession(args []any) (any, error) {
	id := int64(intArg(args, 0))
	b.mu.Lock()
	sess := b.sessions[id]
	delete(b.sessions, id)
	b.mu.Unlock()
	if sess == nil {
		return nil, nil
	}
	b.pool(func() {
		_ = sess.cc.Close()
		_ = sess.conn.Close()
		b.loop.Post(func() { b.loop.Unref() })
	})
	return nil, nil
}
