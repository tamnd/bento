package node

import (
	"encoding/base64"
	"net"
	"strconv"
	"sync"

	"github.com/tamnd/bento/pkg/engine"
)

// dgramBridgeState backs node:dgram (UDP). UDP is connectionless, so unlike net
// there is no handshake or stream: a socket is one net.UDPConn that reads
// datagrams in a loop and writes them on demand. JavaScript holds an integer
// socket id; Go owns the conn and drives the message, listening, and close
// events back through the __bento_dgram_ dispatch globals.
type dgramBridgeState struct {
	netBridge
	mu      sync.Mutex
	nextID  int64
	sockets map[int64]*udpSocket
}

// udpSocket is one datagram socket. conn is nil until the first bind or send
// (Node binds lazily on send), and network is udp4 or udp6.
type udpSocket struct {
	id      int64
	network string
	conn    *net.UDPConn
	once    sync.Once
}

func installDgram(eng engine.Engine, loop LoopHost) error {
	d := &dgramBridgeState{
		netBridge: netBridge{eng: eng, loop: loop},
		sockets:   map[int64]*udpSocket{},
	}
	for name, fn := range d.hostFuncs() {
		if err := eng.Register(name, fn); err != nil {
			return err
		}
	}
	return nil
}

func (d *dgramBridgeState) hostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_dgram_create":       d.create,
		"__bento_dgram_bind":         d.bind,
		"__bento_dgram_send":         d.send,
		"__bento_dgram_close":        d.close,
		"__bento_dgram_setBroadcast": d.setBroadcast,
	}
}

// create allocates a socket id and records its address family. The conn opens
// lazily on the first bind or send.
func (d *dgramBridgeState) create(args []any) (any, error) {
	network := str(args, 0)
	if network != "udp6" {
		network = "udp4"
	}
	d.mu.Lock()
	d.nextID++
	id := d.nextID
	d.sockets[id] = &udpSocket{id: id, network: network}
	d.mu.Unlock()
	return id, nil
}

func (d *dgramBridgeState) lookup(id int64) *udpSocket {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sockets[id]
}

// openConn binds the socket's conn to a local address the first time it is
// needed, takes the loop reference for the socket's lifetime, and starts the
// read loop. It runs inline on the loop goroutine (host functions do), so the
// AddRef is safe without a Post. explicitBind marks whether to emit listening,
// which only an explicit socket.bind does, not the lazy bind a send triggers.
func (d *dgramBridgeState) openConn(sock *udpSocket, laddr *net.UDPAddr, explicitBind bool) error {
	var openErr error
	sock.once.Do(func() {
		conn, err := net.ListenUDP(sock.network, laddr)
		if err != nil {
			openErr = err
			return
		}
		sock.conn = conn
		d.loop.AddRef()
		d.startReadLoop(sock)
	})
	if openErr != nil {
		return openErr
	}
	if explicitBind && sock.conn != nil {
		local := sock.conn.LocalAddr().(*net.UDPAddr)
		d.emit("__bento_dgram_dispatchListening", sock.id, int64(local.Port), local.IP.String())
	}
	return nil
}

func (d *dgramBridgeState) bind(args []any) (any, error) {
	sock := d.lookup(int64(intArg(args, 0)))
	if sock == nil {
		return nil, nil
	}
	port := intArg(args, 1)
	host := str(args, 2)
	laddr := &net.UDPAddr{Port: port}
	if host != "" {
		laddr.IP = net.ParseIP(host)
	}
	if err := d.openConn(sock, laddr, true); err != nil {
		d.emit("__bento_dgram_dispatchError", sock.id, errCode(err), err.Error())
	}
	return nil, nil
}

// send writes one datagram. If the socket has not been bound it binds lazily to
// an ephemeral port first, matching Node. The write runs on a pool goroutine
// because a full send buffer can block, and the optional completion callback is
// invoked back on the loop keyed by sendID (0 means no callback).
func (d *dgramBridgeState) send(args []any) (any, error) {
	sock := d.lookup(int64(intArg(args, 0)))
	if sock == nil {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(str(args, 1))
	if err != nil {
		return nil, nil
	}
	port := intArg(args, 2)
	host := str(args, 3)
	sendID := int64(intArg(args, 4))

	if err := d.openConn(sock, &net.UDPAddr{}, false); err != nil {
		d.dispatchSend(sendID, sock.id, err)
		return nil, nil
	}
	conn := sock.conn
	d.pool(func() {
		addr, rerr := net.ResolveUDPAddr(sock.network, net.JoinHostPort(host, strconv.Itoa(port)))
		if rerr != nil {
			d.dispatchSend(sendID, sock.id, rerr)
			return
		}
		_, werr := conn.WriteToUDP(data, addr)
		d.dispatchSend(sendID, sock.id, werr)
	})
	return nil, nil
}

// dispatchSend reports a send completion. It always fires the socket error event
// on failure so an error is observable even without a per-send callback, and
// fires the keyed send callback when the caller supplied one.
func (d *dgramBridgeState) dispatchSend(sendID, socketID int64, err error) {
	if err != nil {
		d.emit("__bento_dgram_dispatchError", socketID, errCode(err), err.Error())
	}
	if sendID != 0 {
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		d.emit("__bento_dgram_dispatchSend", sendID, msg)
	}
}

func (d *dgramBridgeState) setBroadcast(args []any) (any, error) {
	sock := d.lookup(int64(intArg(args, 0)))
	if sock == nil || sock.conn == nil {
		return nil, nil
	}
	// net.UDPConn has no direct SetBroadcast; the flag is honored at the socket
	// option level and is a no-op for the common loopback and unicast cases bento
	// exercises. Left as a recorded seam for the multicast slice.
	return nil, nil
}

func (d *dgramBridgeState) close(args []any) (any, error) {
	sock := d.lookup(int64(intArg(args, 0)))
	if sock == nil {
		return nil, nil
	}
	if sock.conn != nil {
		_ = sock.conn.Close()
		return nil, nil
	}
	// Never bound: nothing to tear down, so emit close directly and forget it.
	d.mu.Lock()
	delete(d.sockets, sock.id)
	d.mu.Unlock()
	d.emit("__bento_dgram_dispatchClose", sock.id)
	return nil, nil
}

// startReadLoop reads datagrams and posts each one as a message event with the
// sender's address. One copy per datagram hands JavaScript its own buffer. When
// the conn closes the loop ends, the socket is forgotten, its loop reference is
// dropped, and close is emitted.
func (d *dgramBridgeState) startReadLoop(sock *udpSocket) {
	conn := sock.conn
	d.pool(func() {
		buf := make([]byte, 65535) // max UDP payload
		for {
			read, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			msg := make([]byte, read)
			copy(msg, buf[:read])
			d.emit("__bento_dgram_dispatchMessage", sock.id, base64.StdEncoding.EncodeToString(msg), rinfoOf(addr))
		}
		d.mu.Lock()
		delete(d.sockets, sock.id)
		d.mu.Unlock()
		d.loop.Post(func() { d.loop.Unref() })
		d.emit("__bento_dgram_dispatchClose", sock.id)
	})
}

// rinfoOf builds the remote address info Node passes as the message event's
// second argument.
func rinfoOf(addr *net.UDPAddr) string {
	// size is filled in on the JavaScript side from the datagram length.
	type rinfo struct {
		Address string `json:"address"`
		Family  string `json:"family"`
		Port    int    `json:"port"`
	}
	fam := "IPv4"
	if addr.IP.To4() == nil {
		fam = "IPv6"
	}
	return jsonString(rinfo{Address: addr.IP.String(), Family: fam, Port: addr.Port})
}
