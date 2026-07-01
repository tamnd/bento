// https implements node:https on top of node:http. The client is a thin default
// over http.request because the Go client already speaks TLS: it drives Go's
// http.DefaultClient, which picks TLS from the https scheme, so an https URL just
// works. The server is an http.Server subclass whose bind hook hands the cert and
// key to the TLS listener instead of the plain one, reusing the entire request
// and response path underneath.

__bento_defineModule("https", function (module, exports, require) {
  "use strict";

  const http = require("http");

  // pemString accepts the cert and key as a string or a Buffer, matching how
  // callers read them off disk, and hands Go a plain PEM string.
  function pemString(value) {
    if (value == null) return "";
    if (Buffer.isBuffer(value)) return value.toString("utf8");
    if (Array.isArray(value)) return value.map(pemString).join("\n");
    return String(value);
  }

  class Server extends http.Server {
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
      __bento_http_listenTLS(this._id, port, host, this._tlsCert, this._tlsKey);
    }
  }

  function createServer(options, handler) {
    return new Server(options, handler);
  }

  // request and get default the protocol to https and otherwise reuse the http
  // client wholesale. A url string carries its own scheme, so it passes straight
  // through; an options object without one gets https.
  function withHTTPS(input, extra) {
    if (input && typeof input === "object" && typeof input.href !== "string" && !input.protocol) {
      input = Object.assign({ protocol: "https:" }, input);
    }
    return [input, extra];
  }

  function request(input, extra, callback) {
    if (typeof extra === "function") {
      callback = extra;
      extra = undefined;
    }
    const normalized = withHTTPS(input, extra);
    return http.request(normalized[0], normalized[1], callback);
  }

  function get(input, extra, callback) {
    const req = request(input, extra, callback);
    req.end();
    return req;
  }

  module.exports = {
    Server: Server,
    createServer: createServer,
    request: request,
    get: get,
    globalAgent: {},
  };
});
