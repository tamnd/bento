// tls implements node:tls: TLSSocket, Server, connect, and createServer over the
// Go networking bridge. It shares the connection machinery with node:net (the
// same read and write pumps, the same write/end/destroy host functions keyed by a
// shared connection id) and differs only in the transport: the Go side dials or
// accepts with TLS. Its own __bento_tls_dispatch* globals and registries keep tls
// sockets separate from plaintext net sockets.

__bento_defineModule("tls", function (module, exports, require) {
  "use strict";

  const EventEmitter = require("events");
  const net = require("net");

  const servers = Object.create(null); // serverId -> Server
  const sockets = Object.create(null); // connId -> TLSSocket

  // pemString accepts a cert or key as a string or Buffer, matching how callers
  // read them off disk, and yields the PEM text Go parses.
  function pemString(value) {
    if (value == null) return "";
    if (Buffer.isBuffer(value)) return value.toString("utf8");
    if (Array.isArray(value)) return value.map(pemString).join("\n");
    return String(value);
  }

  // TLSSocket reuses net.Socket wholesale: its writes, half-close, and destroy go
  // through the shared host functions keyed by the connection id. Only the extra
  // TLS-specific flags and the tls dispatch routing are added here.
  class TLSSocket extends net.Socket {
    constructor(connId) {
      super(connId);
      this.encrypted = true;
      this.authorized = true;
    }
    getPeerCertificate() {
      return {};
    }
  }

  // connect covers tls.connect(port[, host][, options][, cb]) and
  // tls.connect(options[, cb]). rejectUnauthorized defaults to true, matching
  // Node, and crosses to Go as 1 or 0.
  function connect() {
    const args = Array.prototype.slice.call(arguments);
    let listener;
    if (typeof args[args.length - 1] === "function") listener = args.pop();

    let port = 0;
    let host = "localhost";
    let options = {};
    let i = 0;
    if (typeof args[i] === "number") {
      port = args[i++];
      if (typeof args[i] === "string") host = args[i++];
    }
    if (args[i] && typeof args[i] === "object") options = args[i];
    if (options.port != null) port = options.port;
    if (options.host) host = options.host;
    const rejectUnauthorized = options.rejectUnauthorized === false ? 0 : 1;

    const connId = __bento_tls_connect(port | 0, host, rejectUnauthorized);
    const socket = new TLSSocket(connId);
    socket._connecting = true;
    sockets[connId] = socket;
    if (listener) socket.once("secureConnect", listener);
    return socket;
  }

  class Server extends EventEmitter {
    constructor(options, handler) {
      super();
      if (typeof options === "function") {
        handler = options;
        options = {};
      }
      options = options || {};
      this._id = __bento_tls_createServer();
      servers[this._id] = this;
      this.listening = false;
      this._address = null;
      this._tlsKey = pemString(options.key);
      this._tlsCert = pemString(options.cert);
      if (handler) this.on("secureConnection", handler);
    }
    listen() {
      const args = Array.prototype.slice.call(arguments);
      let cb;
      if (typeof args[args.length - 1] === "function") cb = args.pop();
      let port = 0;
      let host = "";
      if (args.length && typeof args[0] === "object" && args[0] !== null) {
        port = args[0].port | 0;
        host = args[0].host || "";
      } else {
        if (args.length >= 1 && args[0] !== undefined) port = args[0] | 0;
        if (args.length >= 2 && typeof args[1] === "string") host = args[1];
      }
      if (cb) this.once("listening", cb);
      __bento_tls_listen(this._id, port, host, this._tlsCert, this._tlsKey);
      return this;
    }
    address() {
      return this._address;
    }
    close(cb) {
      if (cb) this.once("close", cb);
      __bento_tls_closeServer(this._id);
      return this;
    }
  }

  function createServer(options, handler) {
    return new Server(options, handler);
  }

  globalThis.__bento_tls_dispatchListening = function (serverId, port, address) {
    const server = servers[serverId];
    if (!server) return;
    server.listening = true;
    server._address = { address: address, port: port, family: address.indexOf(":") >= 0 ? "IPv6" : "IPv4" };
    server.emit("listening");
  };

  globalThis.__bento_tls_dispatchConnection = function (serverId, connId, infoJSON) {
    const server = servers[serverId];
    if (!server) return;
    const socket = new TLSSocket(connId);
    socket._applyInfo(JSON.parse(infoJSON));
    sockets[connId] = socket;
    server.emit("secureConnection", socket);
  };

  globalThis.__bento_tls_dispatchConnect = function (connId, infoJSON) {
    const socket = sockets[connId];
    if (!socket) return;
    socket._connecting = false;
    socket._applyInfo(JSON.parse(infoJSON));
    socket.emit("secureConnect");
    socket.emit("connect");
  };

  globalThis.__bento_tls_dispatchData = function (connId, b64) {
    const socket = sockets[connId];
    if (!socket) return;
    const buf = Buffer.from(b64, "base64");
    socket.bytesRead += buf.length;
    socket.push(buf);
  };

  globalThis.__bento_tls_dispatchEnd = function (connId) {
    const socket = sockets[connId];
    if (!socket) return;
    socket.push(null);
  };

  globalThis.__bento_tls_dispatchClose = function (connId) {
    const socket = sockets[connId];
    if (!socket) return;
    delete sockets[connId];
    socket.emit("close");
  };

  globalThis.__bento_tls_dispatchError = function (connId, message) {
    const socket = sockets[connId];
    if (!socket) return;
    delete sockets[connId];
    socket.emit("error", new Error(message));
  };

  globalThis.__bento_tls_dispatchServerError = function (serverId, code, message) {
    const server = servers[serverId];
    if (!server) return;
    const err = new Error(message);
    if (code) err.code = code;
    server.emit("error", err);
  };

  globalThis.__bento_tls_dispatchServerClose = function (serverId) {
    const server = servers[serverId];
    if (!server) return;
    server.listening = false;
    delete servers[serverId];
    server.emit("close");
  };

  module.exports = {
    TLSSocket: TLSSocket,
    Server: Server,
    connect: connect,
    createServer: createServer,
  };
});
