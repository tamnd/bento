package node

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
)

// clientSend performs one outbound HTTP request. JavaScript owns the client id
// and its registry, so Go keeps no per-request state: it does the round trip on a
// pool goroutine and streams the response back through the client dispatch
// globals, keyed by the id JavaScript passed in.
//
// The request body is buffered rather than streamed. ClientRequest collects its
// writes and hands the whole body here on end, which covers the common client and
// fetch cases; a streaming request body over an io.Pipe is a later refinement.
func (h *httpBridge) clientSend(args []any) (any, error) {
	id := int64(intArg(args, 0))
	method := str(args, 1)
	rawURL := str(args, 2)
	headers := decodeHeaderPairs(str(args, 3))
	var body []byte
	if encoded := str(args, 4); encoded != "" {
		if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			body = decoded
		}
	}

	// The request holds the loop open until it settles. AddRef runs here on the
	// loop goroutine; the matching Unref is posted back from the pool goroutine.
	h.loop.AddRef()
	h.pool(func() {
		defer h.loop.Post(func() { h.loop.Unref() })

		var reader io.Reader
		if len(body) > 0 {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, rawURL, reader)
		if err != nil {
			h.emit("__bento_http_dispatchClientError", id, err.Error())
			return
		}
		for _, p := range headers {
			req.Header.Add(p[0], p[1])
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			h.emit("__bento_http_dispatchClientError", id, err.Error())
			return
		}
		h.emit("__bento_http_dispatchClientResponse", id, buildResponseInfo(resp))

		buf := make([]byte, 64*1024)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				h.emit("__bento_http_dispatchClientData", id, base64.StdEncoding.EncodeToString(buf[:n]))
			}
			if rerr != nil {
				_ = resp.Body.Close()
				if rerr == io.EOF {
					h.emit("__bento_http_dispatchClientEnd", id)
				} else {
					h.emit("__bento_http_dispatchClientError", id, rerr.Error())
				}
				return
			}
		}
	})
	return nil, nil
}
