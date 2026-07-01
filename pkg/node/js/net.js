// net implements the node:net TCP surface: Server and Socket over the Go
// networking bridge (pkg/node/net.go). Socket is a Duplex stream, so data flows
// through the same read and write machinery the rest of the stream module uses.
// The Go side owns the real listener and connections and drives this module
// through the __bento_net_dispatch* globals; this side drives writes back through
// the __bento_net_* host functions, all keyed by integer ids.

__bento_defineModule("net", function (module, exports, require) {
  "use strict";

  const EventEmitter = require("events");
  const stream = require("stream");

  const servers = Object.create(null); // serverId -> Server
  const sockets = Object.create(null); // connId -> Socket

  // Socket is a Duplex over one connection id. Reads arrive as pushed data and
  // writes are queued to Go in order. Both inbound (server) and outbound (connect)
  // connections use it; an outbound socket mints its own id before dialing.
  class Socket extends stream.Duplex {
    constructor(connId) {
      super();
      this._connId = connId;
      this._connecting = false;
      this.remoteAddress = undefined;
      this.remotePort = undefined;
      this.localAddress = undefined;
      this.localPort = undefined;
      this.bytesRead = 0;
      this.bytesWritten = 0;
    }
    _applyInfo(info) {
      this.remoteAddress = info.remoteAddress;
      this.remotePort = info.remotePort;
      this.localAddress = info.localAddress;
      this.localPort = info.localPort;
    }
    _read() {}
    _write(chunk, encoding, callback) {
      const buf = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk, encoding || "utf8");
      this.bytesWritten += buf.length;
      __bento_net_write(this._connId, buf.toString("base64"));
      callback();
    }
    _final(callback) {
      __bento_net_end(this._connId);
      callback();
    }
    destroy(err) {
      __bento_net_destroy(this._connId);
      if (err) this.emit("error", err);
      return this;
    }
    setNoDelay() {
      return this;
    }
    setKeepAlive() {
      return this;
    }
    setTimeout(_msecs, callback) {
      if (callback) this.on("timeout", callback);
      return this;
    }
    address() {
      return { address: this.localAddress, port: this.localPort, family: "IPv4" };
    }
    ref() {
      return this;
    }
    unref() {
      return this;
    }
  }

  // connect accepts every net.connect shape: connect(port[, host][, cb]),
  // connect(options[, cb]). The trailing function, if any, is the connect
  // listener.
  function connect() {
    const args = Array.prototype.slice.call(arguments);
    let listener;
    if (typeof args[args.length - 1] === "function") listener = args.pop();

    let port = 0;
    let host = "localhost";
    const first = args[0];
    if (first && typeof first === "object") {
      port = first.port;
      host = first.host || host;
    } else {
      port = first;
      if (typeof args[1] === "string") host = args[1];
    }

    // Go mints and returns the connection id so it never collides with a
    // server-accepted or tls connection.
    const connId = __bento_net_connect(port | 0, host);
    const socket = new Socket(connId);
    socket._connecting = true;
    sockets[connId] = socket;
    if (listener) socket.once("connect", listener);
    return socket;
  }

  class Server extends EventEmitter {
    constructor(options, connectionListener) {
      super();
      if (typeof options === "function") {
        connectionListener = options;
        options = {};
      }
      this._id = __bento_net_createServer();
      servers[this._id] = this;
      this.listening = false;
      this._address = null;
      if (connectionListener) this.on("connection", connectionListener);
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
      __bento_net_listen(this._id, port, host);
      return this;
    }
    address() {
      return this._address;
    }
    close(cb) {
      if (cb) this.once("close", cb);
      __bento_net_closeServer(this._id);
      return this;
    }
  }

  function createServer(options, connectionListener) {
    return new Server(options, connectionListener);
  }

  globalThis.__bento_net_dispatchListening = function (serverId, port, address) {
    const server = servers[serverId];
    if (!server) return;
    server.listening = true;
    server._address = { address: address, port: port, family: address.indexOf(":") >= 0 ? "IPv6" : "IPv4" };
    server.emit("listening");
  };

  globalThis.__bento_net_dispatchConnection = function (serverId, connId, infoJSON) {
    const server = servers[serverId];
    if (!server) return;
    const socket = new Socket(connId);
    socket._applyInfo(JSON.parse(infoJSON));
    sockets[connId] = socket;
    server.emit("connection", socket);
  };

  globalThis.__bento_net_dispatchConnect = function (connId, infoJSON) {
    const socket = sockets[connId];
    if (!socket) return;
    socket._connecting = false;
    socket._applyInfo(JSON.parse(infoJSON));
    socket.emit("connect");
  };

  globalThis.__bento_net_dispatchData = function (connId, b64) {
    const socket = sockets[connId];
    if (!socket) return;
    const buf = Buffer.from(b64, "base64");
    socket.bytesRead += buf.length;
    socket.push(buf);
  };

  globalThis.__bento_net_dispatchEnd = function (connId) {
    const socket = sockets[connId];
    if (!socket) return;
    socket.push(null);
  };

  globalThis.__bento_net_dispatchClose = function (connId) {
    const socket = sockets[connId];
    if (!socket) return;
    delete sockets[connId];
    socket.emit("close");
  };

  globalThis.__bento_net_dispatchError = function (connId, message) {
    const socket = sockets[connId];
    if (!socket) return;
    delete sockets[connId];
    socket.emit("error", new Error(message));
  };

  globalThis.__bento_net_dispatchServerError = function (serverId, code, message) {
    const server = servers[serverId];
    if (!server) return;
    const err = new Error(message);
    if (code) err.code = code;
    server.emit("error", err);
  };

  globalThis.__bento_net_dispatchServerClose = function (serverId) {
    const server = servers[serverId];
    if (!server) return;
    server.listening = false;
    delete servers[serverId];
    server.emit("close");
  };

  module.exports = {
    Server: Server,
    Socket: Socket,
    createServer: createServer,
    connect: connect,
    createConnection: connect,
  };
});
