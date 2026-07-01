// fetch implements the WHATWG fetch layer: Headers, Request, Response, and the
// fetch function, all exposed as globals. It is pure JavaScript over the node
// http client (require("http").request), which already handles both http and
// https URLs because the Go side drives Go's http.DefaultClient, and that does
// TLS by scheme. The body helpers (text, json, arrayBuffer, bytes) buffer the
// whole body, which covers the common cases; streaming bodies are a later step.

__bento_defineModule("fetch", function (module, exports, require) {
  "use strict";

  // Header names are compared case-insensitively but iterate in sorted order per
  // the spec. Values are stored as a list per name so append and getSetCookie
  // keep multiple values distinct; get joins them with ", ".
  class Headers {
    constructor(init) {
      this._map = new Map(); // lowercased name -> { name, values: [] }
      if (init == null) return;
      if (init instanceof Headers) {
        for (const [name, value] of init.entries()) this.append(name, value);
      } else if (Array.isArray(init)) {
        for (const pair of init) {
          if (pair.length !== 2) throw new TypeError("Invalid header tuple");
          this.append(pair[0], pair[1]);
        }
      } else if (typeof init === "object") {
        for (const name of Object.keys(init)) this.append(name, init[name]);
      }
    }

    _key(name) {
      return String(name).toLowerCase();
    }

    append(name, value) {
      const key = this._key(name);
      const entry = this._map.get(key);
      if (entry) {
        entry.values.push(String(value));
      } else {
        this._map.set(key, { name: String(name), values: [String(value)] });
      }
    }

    set(name, value) {
      this._map.set(this._key(name), { name: String(name), values: [String(value)] });
    }

    get(name) {
      const entry = this._map.get(this._key(name));
      return entry ? entry.values.join(", ") : null;
    }

    getSetCookie() {
      const entry = this._map.get("set-cookie");
      return entry ? entry.values.slice() : [];
    }

    has(name) {
      return this._map.has(this._key(name));
    }

    delete(name) {
      this._map.delete(this._key(name));
    }

    forEach(callback, thisArg) {
      for (const key of this._sortedKeys()) {
        const entry = this._map.get(key);
        callback.call(thisArg, entry.values.join(", "), key, this);
      }
    }

    _sortedKeys() {
      return Array.from(this._map.keys()).sort();
    }

    *entries() {
      for (const key of this._sortedKeys()) {
        // set-cookie is the one header the spec yields once per value.
        const entry = this._map.get(key);
        if (key === "set-cookie") {
          for (const value of entry.values) yield [key, value];
        } else {
          yield [key, entry.values.join(", ")];
        }
      }
    }

    *keys() {
      for (const pair of this.entries()) yield pair[0];
    }

    *values() {
      for (const pair of this.entries()) yield pair[1];
    }

    [Symbol.iterator]() {
      return this.entries();
    }

    // _toObject flattens to the plain header object the http client wants, with
    // set-cookie kept as an array so multiple cookies are not folded.
    _toObject() {
      const out = Object.create(null);
      for (const key of this._map.keys()) {
        const entry = this._map.get(key);
        out[entry.name] = key === "set-cookie" ? entry.values.slice() : entry.values.join(", ");
      }
      return out;
    }
  }

  // bodyInit normalizes every allowed body input to a Buffer plus the default
  // content-type the spec assigns, so Request and Response share one code path.
  function bodyInit(body) {
    if (body == null) return { buffer: null, type: null };
    if (typeof body === "string") return { buffer: Buffer.from(body, "utf8"), type: "text/plain;charset=UTF-8" };
    if (Buffer.isBuffer(body)) return { buffer: body, type: null };
    if (body instanceof Uint8Array) return { buffer: Buffer.from(body), type: null };
    if (body instanceof ArrayBuffer) return { buffer: Buffer.from(new Uint8Array(body)), type: null };
    if (body instanceof URLSearchParams) {
      return { buffer: Buffer.from(body.toString(), "utf8"), type: "application/x-www-form-urlencoded;charset=UTF-8" };
    }
    return { buffer: Buffer.from(String(body), "utf8"), type: "text/plain;charset=UTF-8" };
  }

  // Body is the shared mixin behind Request and Response. It holds the whole
  // body as a Buffer and exposes the reader methods, each of which rejects once
  // the body has already been consumed, matching the bodyUsed contract.
  class Body {
    constructor(buffer) {
      this._bodyBuffer = buffer || null;
      this.bodyUsed = false;
    }

    _consume() {
      if (this.bodyUsed) return Promise.reject(new TypeError("Body has already been consumed"));
      this.bodyUsed = true;
      return Promise.resolve(this._bodyBuffer || Buffer.alloc(0));
    }

    text() {
      return this._consume().then((buf) => buf.toString("utf8"));
    }

    json() {
      return this.text().then((text) => JSON.parse(text));
    }

    arrayBuffer() {
      return this._consume().then((buf) => buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength));
    }

    bytes() {
      return this._consume().then((buf) => new Uint8Array(buf));
    }
  }

  class Request extends Body {
    constructor(input, init) {
      init = init || {};
      let url;
      let base = {};
      if (input instanceof Request) {
        url = input.url;
        base = { method: input.method, headers: input.headers };
        if (init.body == null && input._bodyBuffer != null && !input.bodyUsed) {
          init = Object.assign({ body: input._bodyBuffer }, init);
        }
      } else {
        url = input instanceof URL ? input.href : String(input);
      }

      const method = (init.method || base.method || "GET").toUpperCase();
      const parsed = bodyInit(init.body);
      super(parsed.buffer);

      this.url = url;
      this.method = method;
      this.headers = new Headers(init.headers || base.headers);
      this.redirect = init.redirect || "follow";
      this.signal = init.signal || null;
      if (parsed.type && !this.headers.has("content-type")) {
        this.headers.set("content-type", parsed.type);
      }
    }

    clone() {
      return new Request(this.url, {
        method: this.method,
        headers: this.headers,
        body: this._bodyBuffer,
        redirect: this.redirect,
      });
    }
  }

  class Response extends Body {
    constructor(body, init) {
      init = init || {};
      const parsed = bodyInit(body);
      super(parsed.buffer);

      this.status = init.status == null ? 200 : init.status | 0;
      this.statusText = init.statusText == null ? "" : String(init.statusText);
      this.headers = new Headers(init.headers);
      this.ok = this.status >= 200 && this.status < 300;
      this.redirected = false;
      this.type = "default";
      this.url = init.url || "";
      if (parsed.type && !this.headers.has("content-type")) {
        this.headers.set("content-type", parsed.type);
      }
    }

    clone() {
      const copy = new Response(this._bodyBuffer, {
        status: this.status,
        statusText: this.statusText,
        headers: this.headers,
        url: this.url,
      });
      copy.redirected = this.redirected;
      copy.type = this.type;
      return copy;
    }

    static json(data, init) {
      init = init || {};
      const headers = new Headers(init.headers);
      if (!headers.has("content-type")) headers.set("content-type", "application/json");
      return new Response(JSON.stringify(data), Object.assign({}, init, { headers: headers }));
    }

    static error() {
      const res = new Response(null, { status: 0 });
      res.type = "error";
      return res;
    }

    static redirect(url, status) {
      status = status == null ? 302 : status;
      const res = new Response(null, { status: status, headers: { Location: String(url) } });
      return res;
    }
  }

  // fetch drives the request through the http client and buffers the whole
  // response into a Response. Go's client follows redirects and does TLS by
  // scheme, so one path serves http and https.
  function fetch(input, init) {
    return new Promise((resolve, reject) => {
      let request;
      try {
        request = new Request(input, init);
      } catch (err) {
        reject(err);
        return;
      }

      const http = require("http");
      const clientReq = http.request(
        request.url,
        { method: request.method, headers: request.headers._toObject() },
        (res) => {
          const chunks = [];
          res.on("data", (chunk) => chunks.push(chunk));
          res.on("end", () => {
            const response = new Response(Buffer.concat(chunks), {
              status: res.statusCode,
              statusText: res.statusMessage || "",
              headers: incomingHeaders(res),
              url: request.url,
            });
            resolve(response);
          });
          res.on("error", reject);
        }
      );

      clientReq.on("error", reject);
      if (request.signal) {
        if (request.signal.aborted) {
          clientReq.destroy();
          reject(abortError());
          return;
        }
        request.signal.addEventListener("abort", () => {
          clientReq.destroy();
          reject(abortError());
        });
      }
      if (request._bodyBuffer && request._bodyBuffer.length) clientReq.write(request._bodyBuffer);
      clientReq.end();
    });
  }

  function abortError() {
    const err = new Error("The operation was aborted");
    err.name = "AbortError";
    return err;
  }

  // incomingHeaders rebuilds a Headers from an IncomingMessage, preserving the
  // set-cookie array the http layer keeps unfolded.
  function incomingHeaders(res) {
    const headers = new Headers();
    const raw = res.headers || {};
    for (const name of Object.keys(raw)) {
      const value = raw[name];
      if (Array.isArray(value)) {
        for (const item of value) headers.append(name, item);
      } else {
        headers.append(name, value);
      }
    }
    return headers;
  }

  module.exports = {
    fetch: fetch,
    Headers: Headers,
    Request: Request,
    Response: Response,
  };
});
