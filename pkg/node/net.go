package node

import (
	"crypto/tls"
	"encoding/base64"
	"maps"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/tamnd/bento/pkg/engine"
)

// netBridgeState backs node:net. It owns the Go listeners and connections and
// maps them to the integer ids the JavaScript side holds, the same callback-by-id
// protocol http uses. A connection carries its own ordered write queue so writes
// from JavaScript reach the socket in order without ever blocking the loop.
type netBridgeState struct {
	netBridge
	mu      sync.Mutex
	nextID  atomic.Int64
	servers map[int64]net.Listener
	conns   map[int64]*netConn
}

// netConn is one live connection. Writes are handed to a single writer goroutine
// through writes so they serialize; end closes writes after draining, and destroy
// tears the socket down immediately. prefix names the dispatch globals this
// connection's events go to (__bento_net_ or __bento_tls_), which is how one set
// of read and write pumps serves both the plaintext and TLS modules.
type netConn struct {
	id     int64
	conn   net.Conn
	writes chan []byte
	once   sync.Once
	prefix string
}

func installNet(eng engine.Engine, loop LoopHost) error {
	n := &netBridgeState{
		netBridge: netBridge{eng: eng, loop: loop},
		servers:   map[int64]net.Listener{},
		conns:     map[int64]*netConn{},
	}
	funcs := n.hostFuncs()
	maps.Copy(funcs, n.tlsHostFuncs())
	for name, fn := range funcs {
		if err := eng.Register(name, fn); err != nil {
			return err
		}
	}
	return nil
}

func (n *netBridgeState) hostFuncs() map[string]HostFunc {
	// Server and connection ids share one space across net and tls, and write,
	// end, and destroy are keyed by that id, so both modules register the same
	// three connection functions here. Only the transport differs.
	return map[string]HostFunc{
		"__bento_net_createServer": n.createServer,
		"__bento_net_listen":       func(a []any) (any, error) { return n.listenImpl("__bento_net_", nil, a) },
		"__bento_net_closeServer":  n.closeServer,
		"__bento_net_connect":      func(a []any) (any, error) { return n.connectImpl("__bento_net_", false, a) },
		"__bento_net_write":        n.write,
		"__bento_net_end":          n.end,
		"__bento_net_destroy":      n.destroy,
	}
}

func (n *netBridgeState) createServer(_ []any) (any, error) {
	id := n.nextID.Add(1)
	n.mu.Lock()
	n.servers[id] = nil
	n.mu.Unlock()
	return id, nil
}

// listenImpl binds a TCP listener and accepts on a pool goroutine. Each accepted
// connection is registered and announced with a connection event, then its read
// and write pumps start. Binding is inline so a bind error surfaces before the
// loop commits to the server. tlsCfg, when set, wraps the listener so accepted
// connections have already completed the handshake; prefix routes every dispatch
// to the net or tls globals.
func (n *netBridgeState) listenImpl(prefix string, tlsCfg *tls.Config, args []any) (any, error) {
	id := int64(intArg(args, 0))
	port := intArg(args, 1)
	host := str(args, 2)

	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		n.emit(prefix+"dispatchServerError", id, errCode(err), err.Error())
		return nil, nil
	}
	if tlsCfg != nil {
		ln = tls.NewListener(ln, tlsCfg)
	}
	n.mu.Lock()
	n.servers[id] = ln
	n.mu.Unlock()

	bound := ln.Addr().(*net.TCPAddr)
	n.loop.AddRef()
	n.emit(prefix+"dispatchListening", id, int64(bound.Port), bound.IP.String())

	n.pool(func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				break
			}
			nc := n.register(conn, prefix)
			n.emit(prefix+"dispatchConnection", id, nc.id, connInfo(conn))
			n.startPumps(nc)
		}
		n.mu.Lock()
		delete(n.servers, id)
		n.mu.Unlock()
		n.loop.Post(func() { n.loop.Unref() })
		n.emit(prefix+"dispatchServerClose", id)
	})
	return nil, nil
}

func (n *netBridgeState) closeServer(args []any) (any, error) {
	id := int64(intArg(args, 0))
	n.mu.Lock()
	ln := n.servers[id]
	n.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	return nil, nil
}

// connectImpl dials an outbound connection. JavaScript mints the connection id up
// front so the connect and later data events can be routed. Success emits connect
// and starts the pumps; failure emits an error. When secure is set it dials TLS,
// verifying the server name unless the caller opted out (rejectUnauthorized
// false), which arrives as arg 3.
func (n *netBridgeState) connectImpl(prefix string, secure bool, args []any) (any, error) {
	// Go mints the connection id and returns it, so every id (client and
	// server-accepted, net and tls) comes from one counter and cannot collide in
	// the shared conns map.
	id := n.nextID.Add(1)
	port := intArg(args, 0)
	host := str(args, 1)
	rejectUnauthorized := intArg(args, 2) != 0

	// Hold the loop open for the dialing window. This ref runs on the loop
	// goroutine here; on success the connection reuses it, on failure it drops.
	n.loop.AddRef()
	n.pool(func() {
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		var conn net.Conn
		var err error
		if secure {
			conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: !rejectUnauthorized})
		} else {
			conn, err = net.Dial("tcp", addr)
		}
		if err != nil {
			n.loop.Post(func() { n.loop.Unref() })
			n.emit(prefix+"dispatchError", id, err.Error())
			return
		}
		nc := n.adopt(id, conn, prefix, false)
		n.emit(prefix+"dispatchConnect", id, connInfo(conn))
		n.startPumps(nc)
	})
	return id, nil
}

func (n *netBridgeState) write(args []any) (any, error) {
	nc := n.lookup(int64(intArg(args, 0)))
	if nc == nil {
		return nil, nil
	}
	if data, err := base64.StdEncoding.DecodeString(str(args, 1)); err == nil {
		select {
		case nc.writes <- data:
		default:
			// The writer goroutine has exited (connection gone); drop the chunk.
		}
	}
	return nil, nil
}

// end closes the write half after queued writes drain, signalled by closing the
// writes channel. The read side stays open so a peer can still reply.
func (n *netBridgeState) end(args []any) (any, error) {
	nc := n.lookup(int64(intArg(args, 0)))
	if nc == nil {
		return nil, nil
	}
	nc.once.Do(func() { close(nc.writes) })
	return nil, nil
}

func (n *netBridgeState) destroy(args []any) (any, error) {
	nc := n.lookup(int64(intArg(args, 0)))
	if nc == nil {
		return nil, nil
	}
	_ = nc.conn.Close()
	return nil, nil
}

// register allocates an id for an inbound connection and records it, taking a
// fresh loop reference for the connection's lifetime.
func (n *netBridgeState) register(conn net.Conn, prefix string) *netConn {
	return n.adopt(n.nextID.Add(1), conn, prefix, true)
}

// adopt records a connection under a given id. When addRef is set it posts a loop
// reference for the connection (inbound path); the connect path passes false
// because it already holds the dialing reference the connection reuses. AddRef
// is posted because adopt may run on an accept pool goroutine, off the loop.
func (n *netBridgeState) adopt(id int64, conn net.Conn, prefix string, addRef bool) *netConn {
	nc := &netConn{id: id, conn: conn, writes: make(chan []byte, 64), prefix: prefix}
	n.mu.Lock()
	n.conns[id] = nc
	n.mu.Unlock()
	if addRef {
		n.loop.Post(func() { n.loop.AddRef() })
	}
	return nc
}

func (n *netBridgeState) lookup(id int64) *netConn {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.conns[id]
}

// startPumps runs the read and write loops for a connection on pool goroutines.
// The read loop turns socket bytes into data and end events; the write loop
// serializes queued writes and half-closes on drain. When the read loop ends the
// connection is torn down and its loop reference dropped.
func (n *netBridgeState) startPumps(nc *netConn) {
	n.pool(func() {
		for data := range nc.writes {
			if _, err := nc.conn.Write(data); err != nil {
				break
			}
		}
		// Both *net.TCPConn and *tls.Conn expose CloseWrite to half-close, which
		// lets a peer see EOF on our side while we keep reading its reply.
		if cw, ok := nc.conn.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	})

	n.pool(func() {
		buf := make([]byte, 64*1024)
		for {
			read, err := nc.conn.Read(buf)
			if read > 0 {
				n.emit(nc.prefix+"dispatchData", nc.id, base64.StdEncoding.EncodeToString(buf[:read]))
			}
			if err != nil {
				break
			}
		}
		n.emit(nc.prefix+"dispatchEnd", nc.id)
		_ = nc.conn.Close()
		n.mu.Lock()
		delete(n.conns, nc.id)
		n.mu.Unlock()
		n.loop.Post(func() { n.loop.Unref() })
		n.emit(nc.prefix+"dispatchClose", nc.id)
	})
}

// connInfo is the address snapshot handed to JavaScript for a socket, marshaled
// as JSON so the net module can populate remoteAddress and friends.
func connInfo(conn net.Conn) string {
	type info struct {
		RemoteAddress string `json:"remoteAddress"`
		RemotePort    int    `json:"remotePort"`
		LocalAddress  string `json:"localAddress"`
		LocalPort     int    `json:"localPort"`
	}
	var out info
	if a, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		out.RemoteAddress, out.RemotePort = a.IP.String(), a.Port
	}
	if a, ok := conn.LocalAddr().(*net.TCPAddr); ok {
		out.LocalAddress, out.LocalPort = a.IP.String(), a.Port
	}
	return jsonString(out)
}
