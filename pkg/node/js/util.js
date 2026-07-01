// util implements the node:util helpers that show up most in real code:
// format/inspect, promisify/callbackify, inherits, types, and deprecate.

__bento_defineModule("util", function (module, exports, require) {
  "use strict";

  function inspect(obj, opts) {
    return __bento_inspect(obj);
  }
  inspect.custom = Symbol.for("nodejs.util.inspect.custom");

  function format(f) {
    const args = Array.prototype.slice.call(arguments);
    if (typeof f !== "string") {
      return args.map((a) => (typeof a === "string" ? a : inspect(a))).join(" ");
    }
    let i = 1;
    let out = f.replace(/%[sdifjoO%]/g, (m) => {
      if (m === "%%") return "%";
      if (i >= args.length) return m;
      const a = args[i++];
      switch (m) {
        case "%s": return typeof a === "bigint" ? a.toString() + "n" : (typeof a === "object" && a !== null ? inspect(a) : String(a));
        case "%d":
        case "%i": return typeof a === "bigint" ? a.toString() + "n" : String(parseInt(a, 10));
        case "%f": return String(parseFloat(a));
        case "%j": try { return JSON.stringify(a); } catch (e) { return "[Circular]"; }
        case "%o":
        case "%O": return inspect(a);
        default: return m;
      }
    });
    for (; i < args.length; i++) {
      const a = args[i];
      out += " " + (typeof a === "string" ? a : inspect(a));
    }
    return out;
  }

  function inherits(ctor, superCtor) {
    ctor.super_ = superCtor;
    Object.setPrototypeOf(ctor.prototype, superCtor.prototype);
  }

  function promisify(fn) {
    if (typeof fn !== "function") throw new TypeError("original must be a function");
    if (fn[promisify.custom]) return fn[promisify.custom];
    function wrapped() {
      const args = Array.prototype.slice.call(arguments);
      const self = this;
      return new Promise((resolve, reject) => {
        args.push(function (err, value) {
          if (err) reject(err);
          else resolve(value);
        });
        fn.apply(self, args);
      });
    }
    Object.setPrototypeOf(wrapped, Object.getPrototypeOf(fn));
    return wrapped;
  }
  promisify.custom = Symbol.for("nodejs.util.promisify.custom");

  function callbackify(fn) {
    if (typeof fn !== "function") throw new TypeError("original must be a function");
    return function () {
      const args = Array.prototype.slice.call(arguments);
      const cb = args.pop();
      fn.apply(this, args).then(
        (value) => cb(null, value),
        (err) => cb(err || new Error("Promise was rejected with a falsy value"))
      );
    };
  }

  function deprecate(fn, msg) {
    let warned = false;
    return function () {
      if (!warned) {
        warned = true;
        if (typeof console !== "undefined") console.error("DeprecationWarning: " + msg);
      }
      return fn.apply(this, arguments);
    };
  }

  const types = {
    isDate: (v) => v instanceof Date,
    isRegExp: (v) => v instanceof RegExp,
    isNativeError: (v) => v instanceof Error,
    isPromise: (v) => v instanceof Promise,
    isMap: (v) => v instanceof Map,
    isSet: (v) => v instanceof Set,
    isArrayBuffer: (v) => v instanceof ArrayBuffer,
    isAnyArrayBuffer: (v) => v instanceof ArrayBuffer,
    isTypedArray: (v) => ArrayBuffer.isView(v) && !(v instanceof DataView),
    isUint8Array: (v) => v instanceof Uint8Array,
    isDataView: (v) => v instanceof DataView,
    isAsyncFunction: (v) => typeof v === "function" && v.constructor && v.constructor.name === "AsyncFunction",
  };

  const TextEncoderRef = typeof TextEncoder !== "undefined" ? TextEncoder : undefined;
  const TextDecoderRef = typeof TextDecoder !== "undefined" ? TextDecoder : undefined;

  module.exports = {
    format: format,
    formatWithOptions: function (opts, f) { return format.apply(null, Array.prototype.slice.call(arguments, 1)); },
    inspect: inspect,
    inherits: inherits,
    promisify: promisify,
    callbackify: callbackify,
    deprecate: deprecate,
    types: types,
    isDeepStrictEqual: function (a, b) { return deepEqual(a, b); },
    TextEncoder: TextEncoderRef,
    TextDecoder: TextDecoderRef,
    isArray: Array.isArray,
    isBuffer: function (v) { return v && v.constructor && v.constructor.name === "Buffer"; },
    debuglog: function () { return function () {}; },
  };

  function deepEqual(a, b) {
    if (a === b) return true;
    if (typeof a !== "object" || typeof b !== "object" || a === null || b === null) return false;
    const ka = Object.keys(a);
    const kb = Object.keys(b);
    if (ka.length !== kb.length) return false;
    for (const k of ka) if (!deepEqual(a[k], b[k])) return false;
    return true;
  }
});
