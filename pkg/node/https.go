package node

import (
	"crypto/tls"
	"net"
	"strconv"
)

// listenTLS is the https.createServer bind path. It reuses the whole http server
// machinery (handler, exchanges, close) and only differs in wrapping the TCP
// listener in a TLS listener built from the cert and key the JavaScript side
// passes as PEM strings. Serve then accepts connections that have already
// completed the handshake, so the request and response flow is identical to
// plain http from there on.
//
// Both listen paths live on the same httpBridge, so a server created by
// https.createServer routes through the same server id space and dispatch
// globals as an http server; only the transport underneath changes.
func (h *httpBridge) listenTLS(args []any) (any, error) {
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

	tlsLn := tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	h.serveListener(srv, id, tlsLn)
	return nil, nil
}
