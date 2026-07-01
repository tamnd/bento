// url implements the WHATWG URL and URLSearchParams classes exposed both as the
// node:url module and as globals. URL parsing itself is delegated to Go
// (net/url, via __bento_url_parse) because getting percent encoding, default
// ports, and base resolution exactly right is not worth reimplementing here.
// This module owns the JavaScript object surface: the getters and setters, the
// searchParams live view, and the query string encoding that URLSearchParams
// defines.

__bento_defineModule("url", function (module, exports, require) {
  "use strict";

  // parse calls into Go and throws the TypeError the WHATWG spec requires on an
  // invalid input, rather than returning a null result.
  function parse(input, base) {
    const raw = __bento_url_parse(String(input), base == null ? "" : String(base));
    const parsed = JSON.parse(raw);
    if (!parsed.ok) {
      throw new TypeError("Invalid URL: " + input);
    }
    return parsed;
  }

  // encodeQuery / decodeQuery follow the application/x-www-form-urlencoded rules
  // URLSearchParams uses: spaces become "+", everything else is percent encoded
  // by encodeURIComponent, and "*", "!", "'", "(", ")" stay literal to match the
  // browser serializer.
  function encodeQuery(str) {
    return encodeURIComponent(str)
      .replace(/%20/g, "+")
      .replace(/[!'()*]/g, function (c) {
        return "%" + c.charCodeAt(0).toString(16).toUpperCase();
      });
  }

  function decodeQuery(str) {
    return decodeURIComponent(String(str).replace(/\+/g, " "));
  }

  class URLSearchParams {
    constructor(init) {
      this._list = [];
      this._url = null; // set when this view belongs to a URL
      if (init == null || init === "") {
        return;
      }
      if (typeof init === "string") {
        this._parse(init);
      } else if (init instanceof URLSearchParams) {
        this._list = init._list.map((pair) => [pair[0], pair[1]]);
      } else if (Array.isArray(init)) {
        for (const pair of init) {
          if (pair.length !== 2) {
            throw new TypeError("Invalid tuple: each pair needs exactly two values");
          }
          this._list.push([String(pair[0]), String(pair[1])]);
        }
      } else if (typeof init === "object") {
        for (const key of Object.keys(init)) {
          this._list.push([key, String(init[key])]);
        }
      } else {
        this._parse(String(init));
      }
    }

    _parse(query) {
      if (query.charCodeAt(0) === 0x3f) query = query.slice(1); // strip leading "?"
      if (query === "") return;
      for (const part of query.split("&")) {
        if (part === "") continue;
        const eq = part.indexOf("=");
        if (eq === -1) {
          this._list.push([decodeQuery(part), ""]);
        } else {
          this._list.push([decodeQuery(part.slice(0, eq)), decodeQuery(part.slice(eq + 1))]);
        }
      }
    }

    // _sync pushes this view's serialization back onto the owning URL so
    // url.search and url.searchParams never disagree.
    _sync() {
      if (this._url) this._url._setSearch(this.toString());
    }

    append(name, value) {
      this._list.push([String(name), String(value)]);
      this._sync();
    }

    delete(name) {
      name = String(name);
      this._list = this._list.filter((pair) => pair[0] !== name);
      this._sync();
    }

    get(name) {
      name = String(name);
      for (const pair of this._list) {
        if (pair[0] === name) return pair[1];
      }
      return null;
    }

    getAll(name) {
      name = String(name);
      return this._list.filter((pair) => pair[0] === name).map((pair) => pair[1]);
    }

    has(name) {
      name = String(name);
      return this._list.some((pair) => pair[0] === name);
    }

    set(name, value) {
      name = String(name);
      value = String(value);
      let found = false;
      const next = [];
      for (const pair of this._list) {
        if (pair[0] !== name) {
          next.push(pair);
        } else if (!found) {
          next.push([name, value]);
          found = true;
        }
      }
      if (!found) next.push([name, value]);
      this._list = next;
      this._sync();
    }

    sort() {
      this._list.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));
      this._sync();
    }

    forEach(callback, thisArg) {
      for (const pair of this._list) {
        callback.call(thisArg, pair[1], pair[0], this);
      }
    }

    keys() {
      return this._list.map((pair) => pair[0])[Symbol.iterator]();
    }

    values() {
      return this._list.map((pair) => pair[1])[Symbol.iterator]();
    }

    entries() {
      return this._list.map((pair) => [pair[0], pair[1]])[Symbol.iterator]();
    }

    [Symbol.iterator]() {
      return this.entries();
    }

    get size() {
      return this._list.length;
    }

    toString() {
      return this._list
        .map((pair) => encodeQuery(pair[0]) + "=" + encodeQuery(pair[1]))
        .join("&");
    }
  }

  class URL {
    constructor(input, base) {
      this._apply(parse(input, base));
      this._search = this.search;
      this._searchParams = new URLSearchParams(this._search);
      this._searchParams._url = this;
    }

    _apply(parts) {
      this._protocol = parts.protocol;
      this._username = parts.username;
      this._password = parts.password;
      this._host = parts.host;
      this._hostname = parts.hostname;
      this._port = parts.port;
      this._pathname = parts.pathname;
      this._search = parts.search;
      this._hash = parts.hash;
      this._origin = parts.origin;
      this._href = parts.href;
    }

    // _reparse rebuilds every component from the current href after a setter
    // changes one piece, so derived fields (href, origin, host) stay consistent.
    _reparse() {
      this._apply(parse(this._href));
      if (this._searchParams) {
        this._searchParams._list = [];
        this._searchParams._parse(this._search);
      }
    }

    _setSearch(serialized) {
      this._search = serialized ? "?" + serialized : "";
      this._href =
        this._origin === "null"
          ? this._rebuildHref()
          : this._rebuildHref();
    }

    _rebuildHref() {
      let auth = "";
      if (this._username) {
        auth = this._username;
        if (this._password) auth += ":" + this._password;
        auth += "@";
      }
      return (
        this._protocol +
        "//" +
        auth +
        this._host +
        this._pathname +
        this._search +
        this._hash
      );
    }

    get href() {
      return this._href;
    }
    set href(value) {
      this._apply(parse(value));
      this._search = this.search;
      this._searchParams._list = [];
      this._searchParams._parse(this._search);
    }

    get protocol() {
      return this._protocol;
    }
    set protocol(value) {
      value = String(value);
      if (value.charCodeAt(value.length - 1) !== 0x3a) value += ":";
      this._href = value + this._href.slice(this._protocol.length);
      this._reparse();
    }

    get username() {
      return this._username;
    }
    set username(value) {
      this._username = String(value);
      this._href = this._rebuildHref();
      this._reparse();
    }

    get password() {
      return this._password;
    }
    set password(value) {
      this._password = String(value);
      this._href = this._rebuildHref();
      this._reparse();
    }

    get host() {
      return this._host;
    }
    set host(value) {
      this._host = String(value);
      this._href = this._rebuildHref();
      this._reparse();
    }

    get hostname() {
      return this._hostname;
    }
    set hostname(value) {
      value = String(value);
      this._host = this._port ? value + ":" + this._port : value;
      this._href = this._rebuildHref();
      this._reparse();
    }

    get port() {
      return this._port;
    }
    set port(value) {
      value = String(value);
      this._port = value;
      this._host = value ? this._hostname + ":" + value : this._hostname;
      this._href = this._rebuildHref();
      this._reparse();
    }

    get pathname() {
      return this._pathname;
    }
    set pathname(value) {
      value = String(value);
      if (value.charCodeAt(0) !== 0x2f) value = "/" + value;
      this._pathname = value;
      this._href = this._rebuildHref();
      this._reparse();
    }

    get search() {
      return this._search;
    }
    set search(value) {
      value = String(value);
      if (value && value.charCodeAt(0) !== 0x3f) value = "?" + value;
      this._search = value;
      this._href = this._rebuildHref();
      this._reparse();
    }

    get hash() {
      return this._hash;
    }
    set hash(value) {
      value = String(value);
      if (value && value.charCodeAt(0) !== 0x23) value = "#" + value;
      this._hash = value;
      this._href = this._rebuildHref();
      this._reparse();
    }

    get origin() {
      return this._origin;
    }

    get searchParams() {
      return this._searchParams;
    }

    toString() {
      return this._href;
    }

    toJSON() {
      return this._href;
    }
  }

  // canParse mirrors the static URL.canParse: true when construction would
  // succeed, without throwing.
  URL.canParse = function (input, base) {
    const raw = __bento_url_parse(String(input), base == null ? "" : String(base));
    return JSON.parse(raw).ok === true;
  };

  module.exports = {
    URL: URL,
    URLSearchParams: URLSearchParams,
    // Legacy node:url helpers layered on WHATWG URL for the common cases.
    parse: function (input) {
      const u = new URL(input);
      return {
        href: u.href,
        protocol: u.protocol,
        host: u.host,
        hostname: u.hostname,
        port: u.port,
        pathname: u.pathname,
        search: u.search,
        query: u.search ? u.search.slice(1) : null,
        hash: u.hash,
      };
    },
    format: function (u) {
      return u instanceof URL ? u.href : String(u);
    },
    fileURLToPath: function (input) {
      const u = input instanceof URL ? input : new URL(input);
      return decodeURIComponent(u.pathname);
    },
    pathToFileURL: function (path) {
      return new URL("file://" + encodeURI(String(path)));
    },
  };
});
