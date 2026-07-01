// http implements the node:http server surface: createServer, Server,
// IncomingMessage, and ServerResponse. It is enough to run an unmodified Express
// or Hono app. The Go side (pkg/node/http.go) owns the real listener and drives
// this module through the __bento_http_dispatch* globals defined here; this side
// drives the response back through the __bento_http_* host functions. The client
// (http.request / http.get) and http.Agent land in a later slice.

__bento_defineModule("http", function (module, exports, require) {
  "use strict";

  const EventEmitter = require("events");
  const stream = require("stream");

  // Registries keyed by the integer ids the Go bridge hands out. The bridge only
  // speaks ids across the boundary, so these turn an id back into its object.
  const servers = Object.create(null); // serverId -> Server
  const requests = Object.create(null); // reqId -> IncomingMessage

  const STATUS_CODES = {
    100: "Continue", 101: "Switching Protocols", 102: "Processing", 103: "Early Hints",
    200: "OK", 201: "Created", 202: "Accepted", 203: "Non-Authoritative Information",
    204: "No Content", 205: "Reset Content", 206: "Partial Content", 207: "Multi-Status",
    208: "Already Reported", 226: "IM Used",
    300: "Multiple Choices", 301: "Moved Permanently", 302: "Found", 303: "See Other",
    304: "Not Modified", 307: "Temporary Redirect", 308: "Permanent Redirect",
    400: "Bad Request", 401: "Unauthorized", 402: "Payment Required", 403: "Forbidden",
    404: "Not Found", 405: "Method Not Allowed", 406: "Not Acceptable",
    407: "Proxy Authentication Required", 408: "Request Timeout", 409: "Conflict",
    410: "Gone", 411: "Length Required", 412: "Precondition Failed",
    413: "Payload Too Large", 414: "URI Too Long", 415: "Unsupported Media Type",
    416: "Range Not Satisfiable", 417: "Expectation Failed", 418: "I'm a Teapot",
    421: "Misdirected Request", 422: "Unprocessable Entity", 425: "Too Early",
    426: "Upgrade Required", 428: "Precondition Required", 429: "Too Many Requests",
    431: "Request Header Fields Too Large", 451: "Unavailable For Legal Reasons",
    500: "Internal Server Error", 501: "Not Implemented", 502: "Bad Gateway",
    503: "Service Unavailable", 504: "Gateway Timeout", 505: "HTTP Version Not Supported",
    507: "Insufficient Storage", 508: "Loop Detected", 511: "Network Authentication Required",
  };

  const METHODS = [
    "ACL", "BIND", "CHECKOUT", "CONNECT", "COPY", "DELETE", "GET", "HEAD", "LINK",
    "LOCK", "M-SEARCH", "MERGE", "MKACTIVITY", "MKCALENDAR", "MKCOL", "MOVE",
    "NOTIFY", "OPTIONS", "PATCH", "POST", "PROPFIND", "PROPPATCH", "PURGE", "PUT",
    "REBIND", "REPORT", "SEARCH", "SOURCE", "SUBSCRIBE", "TRACE", "UNBIND",
    "UNLINK", "UNLOCK", "UNSUBSCRIBE",
  ];

  function httpError(code, message) {
    const err = new Error(message);
    err.code = code;
    return err;
  }

  // IncomingMessage is the readable side of a request. The body is pulled lazily:
  // the first data or end listener triggers _startPump, which tells Go to begin
  // reading the socket so an ignored body never buffers.
  class IncomingMessage extends stream.Readable {
    constructor(reqId, info) {
      super();
      this._reqId = reqId;
      this._pumpStarted = false;
      this.method = info.method;
      this.url = info.url;
      this.httpVersion = info.httpVersion;
      this.httpVersionMajor = info.httpVersionMajor;
      this.httpVersionMinor = info.httpVersionMinor;
      this.headers = info.headers || {};
      this.rawHeaders = info.rawHeaders || [];
      this.trailers = {};
      this.rawTrailers = [];
      this.complete = false;
      this.aborted = false;
      this.socket = {
        remoteAddress: info.remoteAddress,
        remotePort: info.remotePort,
        localAddress: undefined,
        localPort: undefined,
      };
      this.connection = this.socket;
    }
    _read() {
      this._startPump();
    }
    _startPump() {
      // A client response has no reqId: its body is pushed eagerly by Go, so
      // there is nothing to pump on demand.
      if (this._pumpStarted || this._reqId == null) return;
      this._pumpStarted = true;
      __bento_http_resume(this._reqId);
    }
    on(event, listener) {
      const r = super.on(event, listener);
      if (event === "end" || event === "readable") this._startPump();
      return r;
    }
    setTimeout(_msecs, callback) {
      if (callback) this.on("timeout", callback);
      return this;
    }
  }

  // ServerResponse is the writable side. Headers accumulate case-insensitively and
  // flush on the first body write or on end, so setHeader after writeHead is the
  // only ordering that throws, exactly as Node does.
  class ServerResponse extends stream.Writable {
    constructor(reqId) {
      super();
      this._reqId = reqId;
      this._headers = Object.create(null); // lower name -> { name, value }
      this.statusCode = 200;
      this.statusMessage = undefined;
      this.headersSent = false;
      this.finished = false;
      this.sendDate = true;
    }
    setHeader(name, value) {
      if (this.headersSent) {
        throw httpError("ERR_HTTP_HEADERS_SENT", "Cannot set headers after they are sent to the client");
      }
      this._headers[String(name).toLowerCase()] = { name: name, value: value };
      return this;
    }
    getHeader(name) {
      const h = this._headers[String(name).toLowerCase()];
      return h ? h.value : undefined;
    }
    getHeaders() {
      const out = Object.create(null);
      for (const key in this._headers) out[key] = this._headers[key].value;
      return out;
    }
    getHeaderNames() {
      return Object.keys(this._headers);
    }
    hasHeader(name) {
      return String(name).toLowerCase() in this._headers;
    }
    removeHeader(name) {
      delete this._headers[String(name).toLowerCase()];
    }
    writeHead(statusCode, statusMessage, headers) {
      if (this.headersSent) {
        throw httpError("ERR_HTTP_HEADERS_SENT", "Cannot render headers after they are sent to the client");
      }
      if (statusMessage && typeof statusMessage === "object") {
        headers = statusMessage;
        statusMessage = undefined;
      }
      this.statusCode = statusCode;
      if (statusMessage) this.statusMessage = statusMessage;
      if (headers) {
        if (Array.isArray(headers)) {
          for (let i = 0; i < headers.length; i += 2) this.setHeader(headers[i], headers[i + 1]);
        } else {
          for (const key in headers) this.setHeader(key, headers[key]);
        }
      }
      return this;
    }
    writeHeader() {
      return this.writeHead.apply(this, arguments);
    }
    flushHeaders() {
      this._flushHead();
    }
    _flushHead() {
      if (this.headersSent) return;
      this.headersSent = true;
      __bento_http_writeHead(this._reqId, this.statusCode | 0, JSON.stringify(headersToWire(this._headers)));
    }
    _write(chunk, encoding, callback) {
      this._flushHead();
      __bento_http_write(this._reqId, toBase64(chunk, encoding));
      callback();
    }
    _final(callback) {
      this._flushHead();
      __bento_http_end(this._reqId, "");
      this.finished = true;
      callback();
    }
    setTimeout(_msecs, callback) {
      if (callback) this.on("timeout", callback);
      return this;
    }
  }

  // headersToWire flattens the header store into the [name, value] pairs the Go
  // side adds in order. An array value (set-cookie is the common one) becomes one
  // pair per element so multiple values are not folded into a comma list.
  function headersToWire(store) {
    const pairs = [];
    for (const key in store) {
      const entry = store[key];
      const value = entry.value;
      if (Array.isArray(value)) {
        for (const v of value) pairs.push([String(entry.name), String(v)]);
      } else {
        pairs.push([String(entry.name), String(value)]);
      }
    }
    return pairs;
  }

  // toBase64 normalizes a chunk (string or Buffer/Uint8Array) to base64 for the
  // crossing, honoring the write encoding for string chunks.
  function toBase64(chunk, encoding) {
    if (chunk === undefined || chunk === null) return "";
    const buf = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk, encoding || "utf8");
    return buf.toString("base64");
  }

  class Server extends EventEmitter {
    constructor(opts, handler) {
      super();
      if (typeof opts === "function") {
        handler = opts;
        opts = {};
      }
      this._id = __bento_http_createServer();
      servers[this._id] = this;
      this.listening = false;
      this._address = null;
      if (handler) this.on("request", handler);
    }
    listen() {
      const args = Array.prototype.slice.call(arguments);
      let cb;
      if (typeof args[args.length - 1] === "function") cb = args.pop();

      let port = 0;
      let host = "";
      if (args.length && typeof args[0] === "object" && args[0] !== null) {
        const o = args[0];
        port = o.port | 0;
        host = o.host || "";
      } else {
        if (args.length >= 1 && args[0] !== undefined) port = args[0] | 0;
        if (args.length >= 2 && typeof args[1] === "string") host = args[1];
      }
      if (cb) this.once("listening", cb);
      this._bind(port, host);
      return this;
    }
    // _bind is the transport hook listen calls once it has a port and host. The
    // plain server binds TCP; https.Server overrides it to bind TLS with the same
    // server id, so it reuses this whole class unchanged.
    _bind(port, host) {
      __bento_http_listen(this._id, port, host);
    }
    address() {
      return this._address;
    }
    close(cb) {
      if (cb) this.once("close", cb);
      __bento_http_close(this._id);
      return this;
    }
    setTimeout(_msecs, callback) {
      if (callback) this.on("timeout", callback);
      return this;
    }
    ref() {
      return this;
    }
    unref() {
      return this;
    }
  }

  function createServer(opts, handler) {
    return new Server(opts, handler);
  }

  // Client ids are minted here rather than by Go, since JavaScript has to register
  // the request before the round trip so the dispatch callbacks can find it.
  const clientRequests = Object.create(null); // clientId -> ClientRequest
  let nextClientId = 1;

  // normalizeClientOptions accepts every http.request calling shape: a url string,
  // a URL, an options object, or a url plus an options object. It yields a plain
  // { method, url, headers } the request path builds on.
  function normalizeClientOptions(input, extra) {
    let opts = {};
    if (typeof input === "string") {
      opts = urlToOptions(input);
    } else if (input && typeof input.href === "string") {
      opts = urlToOptions(input.href);
    } else if (input && typeof input === "object") {
      opts = Object.assign({}, input);
    }
    if (extra && typeof extra === "object") opts = Object.assign(opts, extra);

    const protocol = opts.protocol || "http:";
    const hostname = opts.hostname || opts.host || "localhost";
    const port = opts.port ? ":" + opts.port : "";
    const path = opts.path || "/";
    const url = opts.url || protocol + "//" + hostname + port + path;
    return { method: (opts.method || "GET").toUpperCase(), url: url, headers: opts.headers || {} };
  }

  function urlToOptions(href) {
    return { url: href };
  }

  // ClientRequest is the writable side of an outbound request. Body writes are
  // buffered and flushed to Go on end, which then streams the response back.
  class ClientRequest extends stream.Writable {
    constructor(input, extra, callback) {
      super();
      if (typeof extra === "function") {
        callback = extra;
        extra = undefined;
      }
      const opts = normalizeClientOptions(input, extra);
      this._id = nextClientId++;
      clientRequests[this._id] = this;
      this.method = opts.method;
      this._url = opts.url;
      this._headers = Object.create(null);
      this._bodyChunks = [];
      this._response = null;
      for (const key in opts.headers) this.setHeader(key, opts.headers[key]);
      if (callback) this.once("response", callback);
    }
    setHeader(name, value) {
      this._headers[String(name).toLowerCase()] = { name: name, value: value };
      return this;
    }
    getHeader(name) {
      const h = this._headers[String(name).toLowerCase()];
      return h ? h.value : undefined;
    }
    removeHeader(name) {
      delete this._headers[String(name).toLowerCase()];
    }
    _write(chunk, encoding, callback) {
      this._bodyChunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk, encoding || "utf8"));
      callback();
    }
    _final(callback) {
      const body = Buffer.concat(this._bodyChunks);
      __bento_http_clientSend(
        this._id,
        this.method,
        this._url,
        JSON.stringify(headersToWire(this._headers)),
        body.length ? body.toString("base64") : ""
      );
      callback();
    }
    abort() {
      this.destroy();
    }
    setTimeout(_msecs, callback) {
      if (callback) this.on("timeout", callback);
      return this;
    }
  }

  function request(input, extra, callback) {
    return new ClientRequest(input, extra, callback);
  }

  function get(input, extra, callback) {
    const req = request(input, extra, callback);
    req.end();
    return req;
  }

  globalThis.__bento_http_dispatchClientResponse = function (id, infoJSON) {
    const req = clientRequests[id];
    if (!req) return;
    const info = JSON.parse(infoJSON);
    const res = new IncomingMessage(null, info);
    res.statusCode = info.statusCode;
    res.statusMessage = info.statusMessage;
    res.httpVersion = info.httpVersion;
    req._response = res;
    req.emit("response", res);
  };

  globalThis.__bento_http_dispatchClientData = function (id, b64) {
    const req = clientRequests[id];
    if (!req || !req._response) return;
    req._response.push(Buffer.from(b64, "base64"));
  };

  globalThis.__bento_http_dispatchClientEnd = function (id) {
    const req = clientRequests[id];
    if (!req) return;
    if (req._response) {
      req._response.complete = true;
      req._response.push(null);
    }
    delete clientRequests[id];
  };

  globalThis.__bento_http_dispatchClientError = function (id, message) {
    const req = clientRequests[id];
    if (!req) return;
    req.emit("error", new Error(message));
    delete clientRequests[id];
  };

  // The Go bridge calls these globals on the loop goroutine. They are the inbound
  // half of the protocol: server lifecycle plus per-request body events.
  globalThis.__bento_http_dispatchListening = function (serverId, port, address) {
    const server = servers[serverId];
    if (!server) return;
    server.listening = true;
    server._address = { address: address, port: port, family: address.indexOf(":") >= 0 ? "IPv6" : "IPv4" };
    server.emit("listening");
  };

  globalThis.__bento_http_dispatchRequest = function (serverId, reqId, infoJSON) {
    const server = servers[serverId];
    if (!server) return;
    const info = JSON.parse(infoJSON);
    const req = new IncomingMessage(reqId, info);
    const res = new ServerResponse(reqId);
    requests[reqId] = req;
    server.emit("request", req, res);
  };

  globalThis.__bento_http_dispatchData = function (reqId, b64) {
    const req = requests[reqId];
    if (!req) return;
    req.push(Buffer.from(b64, "base64"));
  };

  globalThis.__bento_http_dispatchEnd = function (reqId) {
    const req = requests[reqId];
    if (!req) return;
    req.complete = true;
    req.push(null);
    delete requests[reqId];
  };

  globalThis.__bento_http_dispatchReqError = function (reqId, message) {
    const req = requests[reqId];
    if (!req) return;
    req.aborted = true;
    req.emit("aborted");
    req.emit("error", new Error(message));
    delete requests[reqId];
  };

  globalThis.__bento_http_dispatchServerError = function (serverId, code, message) {
    const server = servers[serverId];
    if (!server) return;
    const err = new Error(message);
    if (code) err.code = code;
    server.emit("error", err);
  };

  globalThis.__bento_http_dispatchClose = function (serverId) {
    const server = servers[serverId];
    if (!server) return;
    server.listening = false;
    delete servers[serverId];
    server.emit("close");
  };

  module.exports = {
    STATUS_CODES: STATUS_CODES,
    METHODS: METHODS,
    Server: Server,
    ServerResponse: ServerResponse,
    IncomingMessage: IncomingMessage,
    ClientRequest: ClientRequest,
    createServer: createServer,
    request: request,
    get: get,
  };
});
