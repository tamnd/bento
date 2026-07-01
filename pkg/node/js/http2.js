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

  // pemString accepts a cert or key as a string or Buffer, the two shapes callers
  // get from reading a file, and hands Go a plain PEM string. It matches https.
  function pemString(value) {
    if (value == null) return "";
    if (Buffer.isBuffer(value)) return value.toString("utf8");
    if (Array.isArray(value)) return value.map(pemString).join("\n");
    return String(value);
  }

  // Http2SecureServer is the compatibility server. It is an http.Server whose bind
  // hook advertises h2 over TLS, so the whole request, response, and close path is
  // inherited and only the transport underneath changes. A request handler passed
  // to createSecureServer becomes the "request" listener, exactly as in http.
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
    }
    _bind(port, host) {
      __bento_http_listenH2(this._id, port, host, this._tlsCert, this._tlsKey);
    }
  }

  function createSecureServer(options, handler) {
    return new Http2SecureServer(options, handler);
  }

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
    Http2SecureServer: Http2SecureServer,
    createSecureServer: createSecureServer,
  };
});
