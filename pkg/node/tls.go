package node

import "crypto/tls"

// tlsHostFuncs registers node:tls on the same netBridgeState as node:net. TLS
// connections share the connection id space and the write, end, and destroy host
// functions with net (js/tls.js calls the __bento_net_ ones directly), so only
// the transport-specific pieces live here: the TLS dial and the TLS bind. Every
// event routes through the __bento_tls_ dispatch prefix so js/tls.js sees only
// its own sockets.
func (n *netBridgeState) tlsHostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_tls_createServer": n.createServer,
		"__bento_tls_listen":       n.tlsListen,
		"__bento_tls_closeServer":  n.closeServer,
		"__bento_tls_connect":      func(a []any) (any, error) { return n.connectImpl("__bento_tls_", true, a) },
	}
}

// tlsListen builds the server TLS config from the cert and key PEM strings and
// binds through the shared listen path with a TLS-wrapped listener.
func (n *netBridgeState) tlsListen(args []any) (any, error) {
	id := int64(intArg(args, 0))
	certPEM := str(args, 3)
	keyPEM := str(args, 4)

	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		n.emit("__bento_tls_dispatchServerError", id, "ERR_TLS_CERT", err.Error())
		return nil, nil
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	return n.listenImpl("__bento_tls_", cfg, args)
}
