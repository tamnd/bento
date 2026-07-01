// http2 implements node:http2. Node splits it into a compatibility API, where an
// ordinary http-style handler serves HTTP/2 unchanged, and a core API of sessions
// and streams. This file provides the compatibility secure server: it subclasses
// http.Server and binds through __bento_http_listenH2, so net/http negotiates h2
// over ALPN and drives the same request/response path as http. A handler sees the
// familiar (req, res) with req.httpVersion === "2.0" when the client picked h2.
//
// The core API (http2.connect, sessions, streams, the stream event) lands in a
// later slice on top of golang.org/x/net/http2; the exports here that belong to
// it are declared where they naturally sit so the surface grows in place.

__bento_defineModule("http2", function (module, exports, require) {
  "use strict";

  const http = require("http");
  const stream = require("stream");
  const EventEmitter = require("events");

  // pemString accepts a cert or key as a string or Buffer, the two shapes callers
  // get from reading a file, and hands Go a plain PEM string. It matches https.
  function pemString(value) {
    if (value == null) return "";
    if (Buffer.isBuffer(value)) return value.toString("utf8");
    if (Array.isArray(value)) return value.map(pemString).join("\n");
    return String(value);
  }

  // Http2Session is the connection object behind a stream. On a net/http h2 server
  // each stream arrives as its own handler invocation, so bento does not thread a
  // real per-connection session through; a lightweight session gives the stream a
  // socket, close, and the EventEmitter surface code reaches for. The core client
  // slice, which owns the connection explicitly, carries the full session state.
  class Http2Session extends EventEmitter {
    constructor(socket) {
      super();
      this.socket = socket || {};
      this.encrypted = true;
      this.destroyed = false;
      this.closed = false;
    }
    close(cb) {
      if (cb) this.once("close", cb);
      if (this.closed) return;
      this.closed = true;
      this.emit("close");
    }
    destroy(err) {
      this.destroyed = true;
      if (err) this.emit("error", err);
      this.close();
    }
    ref() {
      return this;
    }
    unref() {
      return this;
    }
  }

  // buildH2Headers rebuilds the request headers in HTTP/2 shape: the ordinary
  // headers plus the request pseudo-headers a stream handler expects to read.
  function buildH2Headers(req) {
    const h = Object.assign(Object.create(null), req.headers);
    h[":method"] = req.method;
    h[":path"] = req.url;
    h[":scheme"] = "https";
    h[":authority"] = req.headers[":authority"] || req.headers.host || req.headers.authority;
    return h;
  }

  // Http2Stream is one HTTP/2 stream as a Duplex, the shape everything streaming in
  // this runtime shares. On the server it wraps the compat request (its readable
  // body) and response (its writable side), so respond maps onto writeHead and the
  // writable end maps onto the response end. Reading the stream pumps the request
  // body through as data events, same as any other readable here.
  class Http2Stream extends stream.Duplex {
    constructor(req, res) {
      super();
      this._req = req;
      this._res = res;
      this._pumped = false;
      this.id = 0;
      this.sentHeaders = undefined;
      this.session = new Http2Session(res.socket);
      req.on("error", (err) => this.destroy(err));
    }
    _read() {
      if (this._pumped) return;
      this._pumped = true;
      this._req.on("data", (chunk) => this.push(chunk));
      this._req.on("end", () => this.push(null));
    }
    _write(chunk, encoding, callback) {
      this._res.write(chunk, encoding, callback);
    }
    _final(callback) {
      this._res.end();
      callback();
    }
    // respond sends the response headers. The :status pseudo-header becomes the
    // status code and the rest become ordinary headers; other pseudo-headers are
    // dropped since they are not valid on a response. endStream closes the send
    // side immediately, for a headers-only response.
    respond(headers, options) {
      const h = headers || {};
      let status = 200;
      const rest = {};
      for (const key in h) {
        if (key === ":status") status = h[key] | 0;
        else if (key.charCodeAt(0) === 58) continue; // ":" pseudo-header
        else rest[key] = h[key];
      }
      this._res.writeHead(status, rest);
      this.sentHeaders = h;
      if (options && options.endStream) this.end();
      return this;
    }
    close(code, cb) {
      if (typeof code === "function") {
        cb = code;
        code = 0;
      }
      if (cb) this.once("close", cb);
      this.end();
      this.session.close();
    }
    setTimeout(_msecs, callback) {
      if (callback) this.on("timeout", callback);
      return this;
    }
  }

  // Http2SecureServer serves both http2 APIs over one h2 transport. The
  // compatibility API is inherited from http.Server: a request handler becomes the
  // "request" listener and sees (req, res) unchanged. The core API is layered on
  // top: whenever a "stream" listener is attached, each request is also surfaced as
  // an Http2Stream with its HTTP/2 headers, so server.on("stream") works exactly as
  // in Node. A given request is answered through one API or the other.
  class Http2SecureServer extends http.Server {
    constructor(options, handler) {
      if (typeof options === "function") {
        handler = options;
        options = {};
      }
      super(handler);
      options = options || {};
      this._tlsKey = pemString(options.key);
      this._tlsCert = pemString(options.cert);
      // Bridge the compat request into a stream event for the core API. This only
      // fires when someone is listening for streams, so a pure compat server is
      // untouched.
      super.on("request", (req, res) => {
        if (this.listenerCount("stream") === 0) return;
        this.emit("stream", new Http2Stream(req, res), buildH2Headers(req), 0);
      });
    }
    _bind(port, host) {
      __bento_http_listenH2(this._id, port, host, this._tlsCert, this._tlsKey);
    }
  }

  function createSecureServer(options, handler) {
    return new Http2SecureServer(options, handler);
  }

  // Client sessions and streams are keyed by the ids JavaScript mints, since it
  // has to register them before Go dials or round-trips so the dispatch callbacks
  // find their target.
  const clientSessions = Object.create(null); // sessionId -> ClientHttp2Session
  const clientStreams = Object.create(null); // streamId -> ClientHttp2Stream
  let nextSessionId = 1;
  let nextStreamId = 1;

  // ClientHttp2Stream is one outbound stream as a Duplex. Its writable side buffers
  // the request body and hands it to Go on end, which round-trips it on the session
  // and streams the response back as a "response" event then data and end, the same
  // readable shape as an http client response.
  class ClientHttp2Stream extends stream.Duplex {
    constructor(session, headers) {
      super();
      this._session = session;
      this._headers = headers;
      this._id = nextStreamId++;
      this._bodyChunks = [];
      this._sent = false;
      this.session = session;
      this.id = this._id;
      this.sentHeaders = headers;
      clientStreams[this._id] = this;
    }
    _read() {}
    _write(chunk, encoding, callback) {
      this._bodyChunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk, encoding || "utf8"));
      callback();
    }
    _final(callback) {
      this._flush();
      callback();
    }
    _flush() {
      if (this._sent) return;
      this._sent = true;
      // The session dispatches immediately if connected, or queues until the
      // connect event, so ending a request before connect resolves still works.
      this._session._send(this);
    }
    close(code, cb) {
      if (typeof code === "function") {
        cb = code;
      }
      if (cb) this.once("close", cb);
      this.destroy();
    }
    setTimeout(_msecs, callback) {
      if (callback) this.on("timeout", callback);
      return this;
    }
  }

  // ClientHttp2Session is one client connection to an authority. request opens a
  // stream on it; requests made before the connect event resolves are queued and
  // flushed once the session is up, so calling session.request right after connect
  // works without waiting.
  class ClientHttp2Session extends EventEmitter {
    constructor(authority, options) {
      super();
      this._id = nextSessionId++;
      this._connected = false;
      this._closed = false;
      this._pending = [];
      this.socket = {};
      this.encrypted = true;
      clientSessions[this._id] = this;
      __bento_http2_connect(this._id, authority, JSON.stringify(options || {}));
    }
    request(headers, options) {
      const stream = new ClientHttp2Stream(this, headers || {});
      if (options && options.endStream) {
        // A body-less request ends its writable side right away.
        queueMicrotask(() => stream.end());
      }
      return stream;
    }
    // _send is the stream's hand-off point. Once the session is connected the
    // request goes to Go at once; before that it waits in the queue drained by the
    // connect dispatch.
    _send(stream) {
      if (this._closed) {
        stream.emit("error", new Error("session is closed"));
        return;
      }
      if (this._connected) {
        this._dispatch(stream);
      } else {
        this._pending.push(stream);
      }
    }
    _dispatch(stream) {
      const body = Buffer.concat(stream._bodyChunks);
      __bento_http2_request(
        this._id,
        stream._id,
        JSON.stringify(headersToPairs(stream._headers)),
        body.length ? body.toString("base64") : ""
      );
    }
    close(cb) {
      if (cb) this.once("close", cb);
      if (this._closed) return;
      this._closed = true;
      __bento_http2_closeSession(this._id);
      delete clientSessions[this._id];
      this.emit("close");
    }
    destroy(err) {
      if (err) this.emit("error", err);
      this.close();
    }
    ref() {
      return this;
    }
    unref() {
      return this;
    }
  }

  function connect(authority, options, listener) {
    if (typeof options === "function") {
      listener = options;
      options = undefined;
    }
    const session = new ClientHttp2Session(authority, options);
    if (listener) session.once("connect", listener);
    return session;
  }

  // headersToPairs flattens a headers object into the [name, value] pairs the Go
  // side reads, keeping pseudo-headers (":method" and friends) in place. An array
  // value becomes one pair per element, matching multi-valued headers.
  function headersToPairs(headers) {
    const pairs = [];
    for (const key in headers) {
      const value = headers[key];
      if (Array.isArray(value)) {
        for (const v of value) pairs.push([String(key), String(v)]);
      } else {
        pairs.push([String(key), String(value)]);
      }
    }
    return pairs;
  }

  globalThis.__bento_http2_dispatchConnect = function (sessionId) {
    const session = clientSessions[sessionId];
    if (!session) return;
    session._connected = true;
    session.emit("connect", session, session.socket);
    const pending = session._pending;
    session._pending = [];
    for (const stream of pending) session._dispatch(stream);
  };

  globalThis.__bento_http2_dispatchSessionError = function (sessionId, message) {
    const session = clientSessions[sessionId];
    if (!session) return;
    const err = new Error(message);
    session.emit("error", err);
    const pending = session._pending;
    session._pending = [];
    for (const stream of pending) stream.emit("error", err);
    delete clientSessions[sessionId];
  };

  globalThis.__bento_http2_dispatchResponse = function (streamId, infoJSON) {
    const stream = clientStreams[streamId];
    if (!stream) return;
    const info = JSON.parse(infoJSON);
    // Rebuild the HTTP/2 response headers: the ordinary headers plus the :status
    // pseudo-header the response event carries.
    const headers = Object.assign(Object.create(null), info.headers || {});
    headers[":status"] = info.statusCode;
    stream.emit("response", headers, 0);
  };

  globalThis.__bento_http2_dispatchData = function (streamId, b64) {
    const stream = clientStreams[streamId];
    if (!stream) return;
    stream.push(Buffer.from(b64, "base64"));
  };

  globalThis.__bento_http2_dispatchEnd = function (streamId) {
    const stream = clientStreams[streamId];
    if (!stream) return;
    stream.push(null);
    delete clientStreams[streamId];
  };

  globalThis.__bento_http2_dispatchStreamError = function (streamId, message) {
    const stream = clientStreams[streamId];
    if (!stream) return;
    stream.emit("error", new Error(message));
    delete clientStreams[streamId];
  };

  // constants mirrors the subset of node:http2 constants real code reaches for:
  // the request and response pseudo-headers, common header names, and the settings
  // and error codes. The full nghttp2 table is large; this covers the practical
  // surface and grows as the core API needs more.
  const constants = {
    HTTP2_HEADER_STATUS: ":status",
    HTTP2_HEADER_METHOD: ":method",
    HTTP2_HEADER_AUTHORITY: ":authority",
    HTTP2_HEADER_SCHEME: ":scheme",
    HTTP2_HEADER_PATH: ":path",
    HTTP2_HEADER_CONTENT_TYPE: "content-type",
    HTTP2_HEADER_CONTENT_LENGTH: "content-length",
    HTTP2_HEADER_SET_COOKIE: "set-cookie",
    HTTP2_HEADER_COOKIE: "cookie",

    HTTP2_METHOD_GET: "GET",
    HTTP2_METHOD_HEAD: "HEAD",
    HTTP2_METHOD_POST: "POST",
    HTTP2_METHOD_PUT: "PUT",
    HTTP2_METHOD_DELETE: "DELETE",

    NGHTTP2_NO_ERROR: 0x00,
    NGHTTP2_PROTOCOL_ERROR: 0x01,
    NGHTTP2_INTERNAL_ERROR: 0x02,
    NGHTTP2_FLOW_CONTROL_ERROR: 0x03,
    NGHTTP2_SETTINGS_TIMEOUT: 0x04,
    NGHTTP2_STREAM_CLOSED: 0x05,
    NGHTTP2_FRAME_SIZE_ERROR: 0x06,
    NGHTTP2_REFUSED_STREAM: 0x07,
    NGHTTP2_CANCEL: 0x08,
    NGHTTP2_COMPRESSION_ERROR: 0x09,
    NGHTTP2_CONNECT_ERROR: 0x0a,
    NGHTTP2_ENHANCE_YOUR_CALM: 0x0b,
    NGHTTP2_INADEQUATE_SECURITY: 0x0c,
    NGHTTP2_HTTP_1_1_REQUIRED: 0x0d,

    NGHTTP2_SETTINGS_HEADER_TABLE_SIZE: 0x01,
    NGHTTP2_SETTINGS_ENABLE_PUSH: 0x02,
    NGHTTP2_SETTINGS_MAX_CONCURRENT_STREAMS: 0x03,
    NGHTTP2_SETTINGS_INITIAL_WINDOW_SIZE: 0x04,
    NGHTTP2_SETTINGS_MAX_FRAME_SIZE: 0x05,
    NGHTTP2_SETTINGS_MAX_HEADER_LIST_SIZE: 0x06,
  };

  module.exports = {
    constants: constants,
    Http2Session: Http2Session,
    Http2Stream: Http2Stream,
    Http2SecureServer: Http2SecureServer,
    ClientHttp2Session: ClientHttp2Session,
    ClientHttp2Stream: ClientHttp2Stream,
    createSecureServer: createSecureServer,
    connect: connect,
  };
});
