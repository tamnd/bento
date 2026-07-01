package node

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
)

// http2 is two APIs over one wire. The compatibility API lets an ordinary
// http-style handler serve HTTP/2 with no changes, and the core API exposes
// sessions and streams as first-class objects. This file backs the compatibility
// secure server, which is the cheap half: net/http already serves HTTP/2 when a
// TLS handshake negotiates h2 over ALPN, presenting each h2 stream to the same
// http.Handler as an ordinary request. So the whole section 4 request/response
// bridge is reused, and the only difference from https is that the TLS config
// advertises h2 and the server runs through ServeTLS, which turns on net/http's
// built-in HTTP/2 support.

// listenH2 binds a secure HTTP/2 server on the shared http bridge. The listener,
// exchange, and close machinery are identical to http and https; a handler sees
// the same IncomingMessage and ServerResponse, with req.httpVersion reporting
// 2.0 when the client negotiated h2 and 1.1 when it fell back over ALPN.
func (h *httpBridge) listenH2(args []any) (any, error) {
	id := int64(intArg(args, 0))
	port := intArg(args, 1)
	host := str(args, 2)
	certPEM := str(args, 3)
	keyPEM := str(args, 4)

	h.mu.Lock()
	srv := h.servers[id]
	h.mu.Unlock()
	if srv == nil {
		return nil, nil
	}

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		h.emit("__bento_http_dispatchServerError", id, "ERR_TLS_CERT", err.Error())
		return nil, nil
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		h.emit("__bento_http_dispatchServerError", id, errCode(err), err.Error())
		return nil, nil
	}

	// NextProtos advertises h2 ahead of http/1.1, and ServeTLS leaves TLSNextProto
	// nil so net/http auto-configures its HTTP/2 server. The client picks the
	// protocol during the handshake, and the same handler serves whichever wins.
	gosrv := &http.Server{Handler: h.handler(id)}
	gosrv.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}
	h.runServer(srv, id, ln, gosrv, func() error { return gosrv.ServeTLS(ln, "", "") })
	return nil, nil
}
