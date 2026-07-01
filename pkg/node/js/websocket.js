// websocket implements the browser-style WebSocket client global over the Go
// framer bridge (pkg/node/ws.go, wsframe.go). Go does the opening handshake and
// runs the RFC 6455 frame protocol on the raw connection; this side exposes the
// EventTarget-style class (onopen/onmessage/onclose/onerror plus
// addEventListener) and drives send and close through the host functions keyed
// by a connection id. It is promoted to globalThis.WebSocket in the bootstrap.

__bento_defineModule("websocket", function (module, exports, require) {
  "use strict";

  const instances = Object.create(null); // connId -> WebSocket

  const CONNECTING = 0;
  const OPEN = 1;
  const CLOSING = 2;
  const CLOSED = 3;

  // event is the minimal Event shape the WebSocket callbacks receive.
  function makeEvent(type, props) {
    const ev = { type: type, target: null };
    if (props) for (const k in props) ev[k] = props[k];
    return ev;
  }

  class WebSocket {
    constructor(url, protocols) {
      this.url = String(url);
      this.readyState = CONNECTING;
      this.binaryType = "arraybuffer";
      this.protocol = "";
      this.bufferedAmount = 0;
      this._listeners = Object.create(null);
      this.onopen = null;
      this.onmessage = null;
      this.onclose = null;
      this.onerror = null;

      let protoHeader = "";
      if (Array.isArray(protocols)) protoHeader = protocols.join(", ");
      else if (typeof protocols === "string") protoHeader = protocols;

      this._id = __bento_ws_connect(this.url, protoHeader);
      instances[this._id] = this;
    }

    addEventListener(type, listener) {
      (this._listeners[type] || (this._listeners[type] = [])).push(listener);
    }
    removeEventListener(type, listener) {
      const list = this._listeners[type];
      if (!list) return;
      const i = list.indexOf(listener);
      if (i >= 0) list.splice(i, 1);
    }
    _dispatch(type, event) {
      event.target = this;
      const on = this["on" + type];
      if (typeof on === "function") on.call(this, event);
      const list = this._listeners[type];
      if (list) for (const fn of list.slice()) fn.call(this, event);
    }

    send(data) {
      if (this.readyState !== OPEN) {
        throw new Error("WebSocket is not open");
      }
      let buf;
      let binary = 0;
      if (typeof data === "string") {
        buf = Buffer.from(data, "utf8");
      } else if (Buffer.isBuffer(data)) {
        buf = data;
        binary = 1;
      } else if (data instanceof ArrayBuffer) {
        buf = Buffer.from(new Uint8Array(data));
        binary = 1;
      } else if (ArrayBuffer.isView(data)) {
        buf = Buffer.from(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
        binary = 1;
      } else {
        buf = Buffer.from(String(data), "utf8");
      }
      __bento_ws_send(this._id, buf.toString("base64"), binary);
    }

    close(code, reason) {
      if (this.readyState === CLOSED || this.readyState === CLOSING) return;
      this.readyState = CLOSING;
      __bento_ws_close(this._id, code | 0, reason ? String(reason) : "");
    }
  }

  WebSocket.CONNECTING = CONNECTING;
  WebSocket.OPEN = OPEN;
  WebSocket.CLOSING = CLOSING;
  WebSocket.CLOSED = CLOSED;
  WebSocket.prototype.CONNECTING = CONNECTING;
  WebSocket.prototype.OPEN = OPEN;
  WebSocket.prototype.CLOSING = CLOSING;
  WebSocket.prototype.CLOSED = CLOSED;

  globalThis.__bento_ws_dispatchOpen = function (connId, protocol) {
    const ws = instances[connId];
    if (!ws) return;
    ws.readyState = OPEN;
    ws.protocol = protocol || "";
    ws._dispatch("open", makeEvent("open"));
  };

  globalThis.__bento_ws_dispatchMessage = function (connId, b64, binary) {
    const ws = instances[connId];
    if (!ws) return;
    const buf = Buffer.from(b64, "base64");
    let data;
    if (binary) {
      if (ws.binaryType === "nodebuffer") {
        data = buf;
      } else {
        // Hand back a standalone ArrayBuffer sized to the payload.
        data = buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
      }
    } else {
      data = buf.toString("utf8");
    }
    ws._dispatch("message", makeEvent("message", { data: data }));
  };

  globalThis.__bento_ws_dispatchError = function (connId, message) {
    const ws = instances[connId];
    if (!ws) return;
    ws._dispatch("error", makeEvent("error", { message: message, error: new Error(message) }));
  };

  globalThis.__bento_ws_dispatchClose = function (connId, code, reason) {
    const ws = instances[connId];
    if (!ws) return;
    delete instances[connId];
    ws.readyState = CLOSED;
    ws._dispatch("close", makeEvent("close", { code: code, reason: reason || "", wasClean: code === 1000 }));
  };

  module.exports = { WebSocket: WebSocket };
});
