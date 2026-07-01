package node

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/tamnd/bento/pkg/engine"
)

// httpBridge backs the node:http server. It owns the Go http.Server instances
// and the in-flight exchanges, and it maps between Go's blocking handler model
// and the callback-by-id protocol the JavaScript side speaks.
//
// The protocol: JavaScript holds a server id (from __bento_http_createServer)
// and, per request, a request id. Go emits lifecycle events by calling JS
// globals (__bento_http_dispatch*) with those ids, and JavaScript drives the
// response by calling Go host functions (writeHead, write, end) with the request
// id. A Go handler goroutine parks on the exchange's done channel while
// JavaScript composes the response on the loop goroutine, so the ResponseWriter
// is only ever touched by one goroutine at a time.
type httpBridge struct {
	netBridge
	mu        sync.Mutex
	nextID    atomic.Int64
	servers   map[int64]*httpServer
	exchanges map[int64]*httpExchange
}

// httpServer is one bound listener and its Go server. gosrv and ln are set once
// listen succeeds and read only on the loop goroutine or the accept goroutine.
type httpServer struct {
	id    int64
	ln    net.Listener
	gosrv *http.Server
}

// httpExchange is one request/response pair in flight. done is closed by the end
// host function to release the parked handler goroutine. bodyPumped guards the
// one-shot body reader so a handler that reads the body twice starts one pump.
type httpExchange struct {
	id         int64
	w          http.ResponseWriter
	r          *http.Request
	done       chan struct{}
	bodyPumped bool
	ended      bool
}

// installHTTP registers the http host functions against an engine and loop. It is
// called from InstallNet; the js/http.js factory supplies the JavaScript half.
func installHTTP(eng engine.Engine, loop LoopHost) error {
	h := &httpBridge{
		netBridge: netBridge{eng: eng, loop: loop},
		servers:   map[int64]*httpServer{},
		exchanges: map[int64]*httpExchange{},
	}
	for name, fn := range h.hostFuncs() {
		if err := eng.Register(name, fn); err != nil {
			return err
		}
	}
	return nil
}

func (h *httpBridge) hostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_http_createServer": h.createServer,
		"__bento_http_listen":       h.listen,
		"__bento_http_close":        h.close,
		"__bento_http_resume":       h.resume,
		"__bento_http_writeHead":    h.writeHead,
		"__bento_http_write":        h.write,
		"__bento_http_end":          h.end,
	}
}

// createServer allocates a server id and records an empty server. The listener
// is not bound until listen, matching Node where createServer does no I/O.
func (h *httpBridge) createServer(_ []any) (any, error) {
	id := h.nextID.Add(1)
	h.mu.Lock()
	h.servers[id] = &httpServer{id: id}
	h.mu.Unlock()
	return id, nil
}

// listen binds the TCP listener and starts accepting. Binding happens inline so
// a bind error (address in use, permission) is reported before the loop commits
// to keeping the server alive. Once bound it takes a loop reference and serves on
// a pool goroutine, then emits the listening event so server.address() is live.
func (h *httpBridge) listen(args []any) (any, error) {
	id := int64(intArg(args, 0))
	port := intArg(args, 1)
	host := str(args, 2)

	h.mu.Lock()
	srv := h.servers[id]
	h.mu.Unlock()
	if srv == nil {
		return nil, nil
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		h.emit("__bento_http_dispatchServerError", id, errCode(err), err.Error())
		return nil, nil
	}
	srv.ln = ln
	gosrv := &http.Server{Handler: h.handler(id)}
	srv.gosrv = gosrv

	bound := ln.Addr().(*net.TCPAddr)
	h.loop.AddRef()
	h.emit("__bento_http_dispatchListening", id, int64(bound.Port), bound.IP.String())

	h.pool(func() {
		serveErr := gosrv.Serve(ln)
		h.loop.Unref()
		if serveErr != nil && serveErr != http.ErrServerClosed {
			h.emit("__bento_http_dispatchServerError", id, "", serveErr.Error())
		}
		h.mu.Lock()
		delete(h.servers, id)
		h.mu.Unlock()
		h.emit("__bento_http_dispatchClose", id)
	})
	return nil, nil
}

// handler is the Go http.Handler for one server. It registers the exchange,
// hands the request metadata to JavaScript, and blocks until end closes done.
// While parked, the JavaScript side owns the ResponseWriter through the write and
// end host functions, which run on the loop goroutine.
func (h *httpBridge) handler(serverID int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := h.nextID.Add(1)
		ex := &httpExchange{id: reqID, w: w, r: r, done: make(chan struct{})}
		h.mu.Lock()
		h.exchanges[reqID] = ex
		h.mu.Unlock()

		h.emit("__bento_http_dispatchRequest", serverID, reqID, buildRequestInfo(r))
		<-ex.done

		h.mu.Lock()
		delete(h.exchanges, reqID)
		h.mu.Unlock()
	})
}

// resume starts the one-shot body pump for a request. The handler triggers it the
// first time it reads the body (a 'data' or 'end' listener, or an explicit read).
// The body is read on a pool goroutine and pushed to JavaScript chunk by chunk,
// with an end event on EOF, so a large upload never blocks the loop.
func (h *httpBridge) resume(args []any) (any, error) {
	reqID := int64(intArg(args, 0))
	h.mu.Lock()
	ex := h.exchanges[reqID]
	if ex == nil || ex.bodyPumped {
		h.mu.Unlock()
		return nil, nil
	}
	ex.bodyPumped = true
	h.mu.Unlock()

	h.pool(func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := ex.r.Body.Read(buf)
			if n > 0 {
				h.emit("__bento_http_dispatchData", reqID, base64.StdEncoding.EncodeToString(buf[:n]))
			}
			if err != nil {
				if err == io.EOF {
					h.emit("__bento_http_dispatchEnd", reqID)
				} else {
					h.emit("__bento_http_dispatchReqError", reqID, err.Error())
				}
				return
			}
		}
	})
	return nil, nil
}

// writeHead flushes the status line and headers. headersJSON is an array of
// [name, value] pairs so multi-valued headers (set-cookie) survive the crossing
// in order and with their original case.
func (h *httpBridge) writeHead(args []any) (any, error) {
	ex := h.exchange(int64(intArg(args, 0)))
	if ex == nil {
		return nil, nil
	}
	status := intArg(args, 1)
	pairs := decodeHeaderPairs(str(args, 2))
	dst := ex.w.Header()
	for _, p := range pairs {
		dst.Add(p[0], p[1])
	}
	ex.w.WriteHeader(status)
	return nil, nil
}

// write sends one body chunk, base64 encoded on the JavaScript side.
func (h *httpBridge) write(args []any) (any, error) {
	ex := h.exchange(int64(intArg(args, 0)))
	if ex == nil {
		return nil, nil
	}
	if data, err := base64.StdEncoding.DecodeString(str(args, 1)); err == nil {
		_, _ = ex.w.Write(data)
	}
	return nil, nil
}

// end writes an optional final chunk and releases the parked handler goroutine.
// It is guarded so a double end (end called after the stream already finished)
// does not close done twice.
func (h *httpBridge) end(args []any) (any, error) {
	ex := h.exchange(int64(intArg(args, 0)))
	if ex == nil {
		return nil, nil
	}
	h.mu.Lock()
	if ex.ended {
		h.mu.Unlock()
		return nil, nil
	}
	ex.ended = true
	h.mu.Unlock()

	if data := str(args, 1); data != "" {
		if b, err := base64.StdEncoding.DecodeString(data); err == nil {
			_, _ = ex.w.Write(b)
		}
	}
	close(ex.done)
	return nil, nil
}

// close shuts the server down gracefully. Shutdown lets in-flight requests finish
// and makes Serve return ErrServerClosed, which drops the loop reference and
// emits the close event from the accept goroutine.
func (h *httpBridge) close(args []any) (any, error) {
	id := int64(intArg(args, 0))
	h.mu.Lock()
	srv := h.servers[id]
	h.mu.Unlock()
	if srv == nil || srv.gosrv == nil {
		return nil, nil
	}
	gosrv := srv.gosrv
	h.pool(func() { _ = gosrv.Shutdown(context.Background()) })
	return nil, nil
}

// exchange looks up an in-flight exchange by request id.
func (h *httpBridge) exchange(id int64) *httpExchange {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exchanges[id]
}
