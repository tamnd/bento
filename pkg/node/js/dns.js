// dns implements node:dns over the Go resolver bridge (pkg/node/dns.go). Each
// call mints a request id, hands it to a host function, and settles when Go
// dispatches __bento_dns_dispatchResult or __bento_dns_dispatchError keyed by
// that id. Both the callback API and dns.promises are built on the same core.

__bento_defineModule("dns", function (module, exports, require) {
  "use strict";

  const pending = Object.create(null); // id -> function(err, result)
  let nextId = 1;

  // call registers a settler for a fresh id, invokes the host function with that
  // id followed by the caller's arguments, and returns the id.
  function call(hostFn, args, settle) {
    const id = nextId++;
    pending[id] = settle;
    hostFn.apply(null, [id].concat(args));
    return id;
  }

  function makeError(code, message) {
    const err = new Error(message || code);
    err.code = code;
    err.errno = code;
    return err;
  }

  globalThis.__bento_dns_dispatchResult = function (id, json) {
    const settle = pending[id];
    if (!settle) return;
    delete pending[id];
    settle(null, JSON.parse(json));
  };

  globalThis.__bento_dns_dispatchError = function (id, code, message) {
    const settle = pending[id];
    if (!settle) return;
    delete pending[id];
    settle(makeError(code, message), null);
  };

  // splitArgs pulls an optional options object and the trailing callback out of
  // an arguments list, matching the (host[, options], callback) shape shared by
  // every resolveX function.
  function splitArgs(args) {
    args = Array.prototype.slice.call(args);
    let cb;
    if (typeof args[args.length - 1] === "function") cb = args.pop();
    let options = {};
    if (args.length && typeof args[args.length - 1] === "object" && args[args.length - 1] !== null) {
      options = args.pop();
    }
    return { positional: args, options: options, cb: cb };
  }

  // lookup(host[, options], cb). options may be a number (family) or an object
  // with family and all. The callback is cb(err, address, family) unless
  // all:true, then cb(err, [{address, family}]).
  function lookup() {
    const parts = splitArgs(arguments);
    const host = parts.positional[0];
    let options = parts.options;
    if (typeof parts.positional[1] === "number") options = { family: parts.positional[1] };
    const family = options.family === 6 ? 6 : options.family === 4 ? 4 : 0;
    const all = options.all === true;
    const cb = parts.cb || function () {};
    call(__bento_dns_lookup, [host, family, all ? 1 : 0], function (err, result) {
      if (err) return cb(err);
      if (all) return cb(null, result);
      cb(null, result.address, result.family);
    });
  }

  // resolver builds a callback-style resolveX from its host function. The result
  // is passed straight through to cb(err, records).
  function resolver(hostFn) {
    return function () {
      const parts = splitArgs(arguments);
      const cb = parts.cb || function () {};
      call(hostFn, [parts.positional[0]], cb);
    };
  }

  const resolve4 = resolver(__bento_dns_resolve4);
  const resolve6 = resolver(__bento_dns_resolve6);
  const resolveMx = resolver(__bento_dns_resolveMx);
  const resolveTxt = resolver(__bento_dns_resolveTxt);
  const resolveSrv = resolver(__bento_dns_resolveSrv);
  const resolveNs = resolver(__bento_dns_resolveNs);
  const resolveCname = resolver(__bento_dns_resolveCname);
  const resolvePtr = resolver(__bento_dns_resolvePtr);
  const reverse = resolver(__bento_dns_reverse);

  // resolve(host[, rrtype], cb) dispatches to the resolveX matching the record
  // type, defaulting to A records like Node.
  const byType = {
    A: resolve4,
    AAAA: resolve6,
    MX: resolveMx,
    TXT: resolveTxt,
    SRV: resolveSrv,
    NS: resolveNs,
    CNAME: resolveCname,
    PTR: resolvePtr,
  };
  function resolve() {
    const args = Array.prototype.slice.call(arguments);
    const cb = typeof args[args.length - 1] === "function" ? args.pop() : function () {};
    const host = args[0];
    const rrtype = typeof args[1] === "string" ? args[1] : "A";
    const fn = byType[rrtype];
    if (!fn) return cb(makeError("ENOTIMP", "unsupported rrtype " + rrtype));
    fn(host, cb);
  }

  // promisify turns a callback-style function into one that returns a promise,
  // used to build the dns.promises face from the same cores. lookup keeps its
  // (address, family) result shape as an object when not in all mode.
  function promisify(fn, isLookup) {
    return function () {
      const args = Array.prototype.slice.call(arguments);
      return new Promise(function (resolveP, rejectP) {
        args.push(function (err) {
          if (err) return rejectP(err);
          if (isLookup) {
            const rest = Array.prototype.slice.call(arguments, 1);
            resolveP(rest.length > 1 ? { address: rest[0], family: rest[1] } : rest[0]);
          } else {
            resolveP(arguments[1]);
          }
        });
        fn.apply(null, args);
      });
    };
  }

  const promises = {
    lookup: promisify(lookup, true),
    resolve: promisify(resolve, false),
    resolve4: promisify(resolve4, false),
    resolve6: promisify(resolve6, false),
    resolveMx: promisify(resolveMx, false),
    resolveTxt: promisify(resolveTxt, false),
    resolveSrv: promisify(resolveSrv, false),
    resolveNs: promisify(resolveNs, false),
    resolveCname: promisify(resolveCname, false),
    resolvePtr: promisify(resolvePtr, false),
    reverse: promisify(reverse, false),
  };

  module.exports = {
    lookup: lookup,
    resolve: resolve,
    resolve4: resolve4,
    resolve6: resolve6,
    resolveMx: resolveMx,
    resolveTxt: resolveTxt,
    resolveSrv: resolveSrv,
    resolveNs: resolveNs,
    resolveCname: resolveCname,
    resolvePtr: resolvePtr,
    reverse: reverse,
    promises: promises,
    ADDRCONFIG: 1024,
    V4MAPPED: 8,
  };
});
