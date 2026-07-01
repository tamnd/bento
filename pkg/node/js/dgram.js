// dgram implements node:dgram (UDP) over the Go socket bridge (pkg/node/dgram.go).
// A dgram.Socket is an EventEmitter, not a stream, because datagrams have
// boundaries. The Go side owns the net.UDPConn and drives message, listening,
// and close events back through the __bento_dgram_dispatch* globals; this side
// drives bind, send, and close through the host functions keyed by a socket id.

__bento_defineModule("dgram", function (module, exports, require) {
  "use strict";

  const EventEmitter = require("events");

  const sockets = Object.create(null); // socketId -> Socket
  const sends = Object.create(null); // sendId -> callback
  let nextSendId = 1;

  class Socket extends EventEmitter {
    constructor(type, listener) {
      super();
      if (type && typeof type === "object") {
        listener = listener || type.recvBufferSize; // ignore extra options
        type = type.type;
      }
      this.type = type === "udp6" ? "udp6" : "udp4";
      this._id = __bento_dgram_create(this.type);
      this._bound = false;
      this._address = null;
      sockets[this._id] = this;
      if (typeof listener === "function") this.on("message", listener);
    }

    // bind(port[, address][, cb]) and bind(options[, cb]). The listening event
    // fires when Go confirms the bind.
    bind() {
      const args = Array.prototype.slice.call(arguments);
      let cb;
      if (typeof args[args.length - 1] === "function") cb = args.pop();
      let port = 0;
      let address = "";
      if (args.length && typeof args[0] === "object" && args[0] !== null) {
        port = args[0].port | 0;
        address = args[0].address || "";
      } else {
        if (args.length >= 1 && args[0] !== undefined) port = args[0] | 0;
        if (args.length >= 2 && typeof args[1] === "string") address = args[1];
      }
      if (cb) this.once("listening", cb);
      __bento_dgram_bind(this._id, port, address);
      return this;
    }

    // send(msg[, offset, length], port[, address][, cb]). bento supports the
    // common shapes: a string or Buffer message, a port, an optional address,
    // and an optional completion callback.
    send() {
      const args = Array.prototype.slice.call(arguments);
      let cb;
      if (typeof args[args.length - 1] === "function") cb = args.pop();
      const msg = args[0];
      let i = 1;
      // Skip the optional offset and length numbers when a length follows.
      if (typeof args[1] === "number" && typeof args[2] === "number") i = 3;
      const port = args[i] | 0;
      const address = typeof args[i + 1] === "string" ? args[i + 1] : "127.0.0.1";

      const buf = Buffer.isBuffer(msg) ? msg : Buffer.from(String(msg), "utf8");
      let sendId = 0;
      if (cb) {
        sendId = nextSendId++;
        sends[sendId] = cb;
      }
      __bento_dgram_send(this._id, buf.toString("base64"), port, address, sendId);
      return this;
    }

    address() {
      return this._address;
    }

    setBroadcast(flag) {
      __bento_dgram_setBroadcast(this._id, flag ? 1 : 0);
      return this;
    }

    // These socket options are accepted for compatibility; the loopback and
    // unicast paths bento exercises do not need them.
    setTTL() {
      return this;
    }
    setMulticastTTL() {
      return this;
    }
    setMulticastLoopback() {
      return this;
    }
    addMembership() {
      return this;
    }
    dropMembership() {
      return this;
    }
    ref() {
      return this;
    }
    unref() {
      return this;
    }

    close(cb) {
      if (cb) this.once("close", cb);
      __bento_dgram_close(this._id);
      return this;
    }
  }

  globalThis.__bento_dgram_dispatchListening = function (socketId, port, address) {
    const socket = sockets[socketId];
    if (!socket) return;
    socket._bound = true;
    socket._address = { address: address, port: port, family: address.indexOf(":") >= 0 ? "IPv6" : "IPv4" };
    socket.emit("listening");
  };

  globalThis.__bento_dgram_dispatchMessage = function (socketId, b64, rinfoJSON) {
    const socket = sockets[socketId];
    if (!socket) return;
    const buf = Buffer.from(b64, "base64");
    const rinfo = JSON.parse(rinfoJSON);
    rinfo.size = buf.length;
    socket.emit("message", buf, rinfo);
  };

  globalThis.__bento_dgram_dispatchSend = function (sendId, message) {
    const cb = sends[sendId];
    if (!cb) return;
    delete sends[sendId];
    cb(message ? new Error(message) : null);
  };

  globalThis.__bento_dgram_dispatchError = function (socketId, code, message) {
    const socket = sockets[socketId];
    if (!socket) return;
    const err = new Error(message);
    if (code) err.code = code;
    socket.emit("error", err);
  };

  globalThis.__bento_dgram_dispatchClose = function (socketId) {
    const socket = sockets[socketId];
    if (!socket) return;
    delete sockets[socketId];
    socket._bound = false;
    socket.emit("close");
  };

  function createSocket(type, listener) {
    return new Socket(type, listener);
  }

  module.exports = {
    Socket: Socket,
    createSocket: createSocket,
  };
});
