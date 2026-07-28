// prelude.js sets up the global environment before user code runs.
//
// It wires the JavaScript side of the host bridge: console, process, timers,
// and a small module registry. The Go side exposes a handful of primitive host
// functions (names prefixed with __bento_) and this file builds the familiar
// Node and browser globals on top of them so existing code feels at home.
//
// Everything here runs in one realm on the event-loop goroutine, so there is no
// concurrency to guard against.

(function () {
  "use strict";

  const g = globalThis;

  // globalThis, global, and self all point at the same object, matching Node
  // and the web platform so feature-detecting code takes the right branch.
  g.global = g;
  g.self = g;

  // ---- value formatting -------------------------------------------------
  // A compact inspector good enough for console output. It is not util.inspect
  // yet, but it handles the common shapes without pulling in the node layer.
  function inspect(value, seen) {
    seen = seen || new Set();
    const t = typeof value;
    if (value === null) return "null";
    if (t === "undefined") return "undefined";
    if (t === "string") return seen.size === 0 ? value : JSON.stringify(value);
    if (t === "number" || t === "boolean" || t === "bigint") return String(value);
    if (t === "symbol") return value.toString();
    if (t === "function") {
      const name = value.name ? ": " + value.name : " (anonymous)";
      return "[Function" + name + "]";
    }
    if (value instanceof Error) {
      return value.stack || value.name + ": " + value.message;
    }
    if (seen.has(value)) return "[Circular]";
    seen.add(value);
    let out;
    if (Array.isArray(value)) {
      out = "[ " + value.map((v) => inspect(v, seen)).join(", ") + " ]";
      if (value.length === 0) out = "[]";
    } else if (value instanceof Map) {
      const parts = [];
      value.forEach((v, k) => parts.push(inspect(k, seen) + " => " + inspect(v, seen)));
      out = "Map(" + value.size + ") { " + parts.join(", ") + " }";
    } else if (value instanceof Set) {
      const parts = [];
      value.forEach((v) => parts.push(inspect(v, seen)));
      out = "Set(" + value.size + ") { " + parts.join(", ") + " }";
    } else {
      const keys = Object.keys(value);
      if (keys.length === 0) {
        out = "{}";
      } else {
        const parts = keys.map((k) => k + ": " + inspect(value[k], seen));
        out = "{ " + parts.join(", ") + " }";
      }
    }
    seen.delete(value);
    return out;
  }
  g.__bento_inspect = inspect;

  function format(args) {
    return Array.prototype.map.call(args, (a) => inspect(a)).join(" ");
  }

  // ---- console ----------------------------------------------------------
  // fd 1 is stdout, fd 2 is stderr. __bento_write is the single Go sink.
  function writeLine(fd, args) {
    __bento_write(fd, format(args) + "\n");
  }
  g.console = {
    log: function () { writeLine(1, arguments); },
    info: function () { writeLine(1, arguments); },
    debug: function () { writeLine(1, arguments); },
    warn: function () { writeLine(2, arguments); },
    error: function () { writeLine(2, arguments); },
    trace: function () { writeLine(2, arguments); },
    dir: function (o) { writeLine(1, [inspect(o)]); },
    assert: function (cond) {
      if (!cond) writeLine(2, ["Assertion failed:"].concat(Array.prototype.slice.call(arguments, 1)));
    },
    // Timers and counters keep just enough state to be useful.
    _times: new Map(),
    _counts: new Map(),
    time: function (label) { this._times.set(label || "default", __bento_now()); },
    timeEnd: function (label) {
      label = label || "default";
      const start = this._times.get(label);
      if (start !== undefined) {
        writeLine(1, [label + ": " + (__bento_now() - start).toFixed(3) + "ms"]);
        this._times.delete(label);
      }
    },
    count: function (label) {
      label = label || "default";
      const n = (this._counts.get(label) || 0) + 1;
      this._counts.set(label, n);
      writeLine(1, [label + ": " + n]);
    },
    group: function () { writeLine(1, arguments); },
    groupEnd: function () {},
    table: function (o) { writeLine(1, [inspect(o)]); },
  };

  // ---- timers -----------------------------------------------------------
  // The JavaScript side owns the callback registry so the Go side only ever
  // deals with integer ids and millisecond delays. Go schedules the fire and
  // calls back into __bento_runTimer.
  const timers = new Map();
  let nextTimerId = 1;

  function makeTimer(cb, delay, repeat, args) {
    if (typeof cb !== "function") {
      throw new TypeError("callback must be a function");
    }
    const id = nextTimerId++;
    const bound = args.length ? () => cb.apply(undefined, args) : cb;
    timers.set(id, bound);
    __bento_setTimer(id, delay | 0, repeat);
    return id;
  }

  g.setTimeout = function (cb, delay) {
    return makeTimer(cb, delay, false, Array.prototype.slice.call(arguments, 2));
  };
  g.setInterval = function (cb, delay) {
    return makeTimer(cb, delay, true, Array.prototype.slice.call(arguments, 2));
  };
  g.setImmediate = function (cb) {
    return makeTimer(cb, 0, false, Array.prototype.slice.call(arguments, 1));
  };
  g.clearTimeout = function (id) {
    if (timers.delete(id)) __bento_clearTimer(id);
  };
  g.clearInterval = g.clearTimeout;
  g.clearImmediate = g.clearTimeout;

  // Called by Go when a scheduled timer is due. repeat controls whether the
  // registry entry survives for the next tick.
  g.__bento_runTimer = function (id, repeat) {
    const fn = timers.get(id);
    if (!fn) return;
    if (!repeat) timers.delete(id);
    fn();
  };

  g.queueMicrotask = function (cb) {
    if (typeof cb !== "function") throw new TypeError("callback must be a function");
    Promise.resolve().then(cb);
  };

  // ---- performance ------------------------------------------------------
  // performance.now() returns milliseconds from a monotonic origin, reading the
  // same host clock the compiled path lowers performance.now() to, so a program
  // timed here and after compilation measure the same way.
  g.performance = {
    now: function () {
      return __bento_perfNow();
    },
  };

  // ---- process ----------------------------------------------------------
  // Boot data (argv, env, platform, cwd) is injected by Go as a JSON string so
  // the bridge stays to plain values.
  const boot = JSON.parse(__bento_boot());
  const process = {
    argv: boot.argv,
    argv0: boot.argv0,
    execPath: boot.execPath,
    env: boot.env,
    platform: boot.platform,
    arch: boot.arch,
    pid: boot.pid,
    version: boot.version,
    versions: boot.versions,
    cwd: function () { return __bento_cwd(); },
    exit: function (code) { __bento_exit(code | 0); },
    nextTick: function (cb) {
      const args = Array.prototype.slice.call(arguments, 1);
      Promise.resolve().then(() => cb.apply(undefined, args));
    },
    hrtime: function (prev) {
      const now = __bento_hrtime();
      const sec = Math.floor(now / 1e9);
      const nsec = now % 1e9;
      if (prev) {
        let ds = sec - prev[0];
        let dn = nsec - prev[1];
        if (dn < 0) { ds -= 1; dn += 1e9; }
        return [ds, dn];
      }
      return [sec, nsec];
    },
    stdout: { write: function (s) { __bento_write(1, String(s)); return true; }, fd: 1 },
    stderr: { write: function (s) { __bento_write(2, String(s)); return true; }, fd: 2 },
    _listeners: new Map(),
    on: function (ev, cb) {
      const list = this._listeners.get(ev) || [];
      list.push(cb);
      this._listeners.set(ev, list);
      return this;
    },
    once: function (ev, cb) { return this.on(ev, cb); },
    off: function (ev, cb) {
      const list = this._listeners.get(ev);
      if (list) this._listeners.set(ev, list.filter((f) => f !== cb));
      return this;
    },
    emit: function (ev) {
      const list = this._listeners.get(ev);
      if (!list) return false;
      const args = Array.prototype.slice.call(arguments, 1);
      list.slice().forEach((f) => f.apply(undefined, args));
      return true;
    },
  };
  process.hrtime.bigint = function () { return BigInt(__bento_hrtime()); };
  g.process = process;

  // ---- module system ----------------------------------------------------
  // Three registries back require(). `resolved` holds already-built exports for
  // eagerly registered modules and the cache of native modules that have run.
  // `factories` holds native core modules as functions that build their exports
  // lazily on first require, matching Node's own lazy core loading. `userCache`
  // holds on-disk modules keyed by their realpath, so a file required twice runs
  // once. User resolution and loading go through the __bento_loadModule host
  // bridge, which drives the Go resolver and transpiler.
  const resolved = new Map();
  const factories = new Map();
  const userCache = Object.create(null);

  function withNodePrefix(name, set) {
    set(name);
    if (name.indexOf("node:") === 0) {
      set(name.slice(5));
    } else {
      set("node:" + name);
    }
  }

  // Eagerly register a fully built module object under its name (and the
  // node: alias). Used for modules that are cheap and always present.
  g.__bento_registerModule = function (name, exportsObj) {
    withNodePrefix(name, (n) => resolved.set(n, exportsObj));
  };

  // Register a native core module as a factory. The factory runs at most once,
  // on first require, and receives (module, exports, require) like a CommonJS
  // module so core modules can require one another.
  g.__bento_defineModule = function (name, factory) {
    withNodePrefix(name, (n) => factories.set(n, factory));
  };

  function loadNative(spec) {
    if (resolved.has(spec)) return resolved.get(spec);
    const factory = factories.get(spec);
    if (!factory) return undefined;
    const mod = { exports: {}, id: spec, loaded: false };
    // Register before running so cyclic core requires see the partial exports.
    withNodePrefix(spec, (n) => resolved.set(n, mod.exports));
    factory(mod, mod.exports, g.require);
    mod.loaded = true;
    withNodePrefix(spec, (n) => resolved.set(n, mod.exports));
    return mod.exports;
  }

  function moduleNotFound(spec) {
    const err = new Error("Cannot find module '" + spec + "'");
    err.code = "MODULE_NOT_FOUND";
    return err;
  }

  // dirnameOf returns the directory portion of a path, a plain string helper used
  // when the host does not supply a directory (data: modules and the like). It
  // splits on either separator, since a Windows path may carry a backslash and a
  // data: URL never does, and the host's answer is preferred over this one
  // wherever there is one.
  function dirnameOf(p) {
    const i = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
    if (i < 0) return ".";
    return i === 0 ? p.slice(0, 1) : p.slice(0, i);
  }

  // compileModule wraps transpiled CommonJS source in the classic Node module
  // wrapper via the Function constructor, so exports, require, module, __filename,
  // and __dirname are the module's own locals rather than shared globals.
  function compileModule(code, filename) {
    try {
      return new Function("exports", "require", "module", "__filename", "__dirname", code);
    } catch (e) {
      if (e instanceof Error) e.message = filename + ": " + e.message;
      throw e;
    }
  }

  // loadUserModule runs an on-disk (or data:) module the host already resolved
  // and, for code, transpiled. The record is cached before the body runs so a
  // circular require sees the partial exports, exactly as Node does.
  function loadUserModule(info) {
    // A module has two spellings. info.path is the module path the host resolves
    // against, slash-separated on every platform. info.filename is the operating
    // system's spelling, which is what Node puts in __filename, module.filename,
    // module.id, and the require.cache key, so that is what this side uses. A
    // data: module has no file, so it falls back to its own name. On Unix the two
    // spellings are the same string; on Windows they are not.
    const filename = info.filename || info.path;
    const cached = userCache[filename];
    if (cached) return cached.exports;

    const record = {
      id: filename,
      filename: filename,
      exports: {},
      loaded: false,
      format: info.format || "commonjs",
    };
    userCache[filename] = record;

    try {
      if (info.kind === "json") {
        record.exports = JSON.parse(info.source);
        record.loaded = true;
        return record.exports;
      }
      const fn = compileModule(info.code, filename);
      const dir = info.dir || dirnameOf(filename);
      fn.call(record.exports, record.exports, requireFrom(info.path, record.format), record, filename, dir);
      record.loaded = true;
      return record.exports;
    } catch (e) {
      // A module that throws while loading must not leave a poisoned half-built
      // entry behind; a later require should try again from scratch.
      delete userCache[filename];
      throw e;
    }
  }

  // requireFrom builds a require function bound to one module's path and format,
  // so relative specifiers resolve against that module's directory. Core modules
  // win first (Node core always beats a same-named package), then the host
  // resolver handles files, packages, and data: URLs.
  function requireFrom(parentPath, parentFormat) {
    function req(spec) {
      const native = loadNative(spec);
      if (native !== undefined) return native;

      const info = JSON.parse(__bento_loadModule(spec, parentPath || "", parentFormat || "commonjs"));
      if (!info.ok) {
        const err = new Error(info.message || "Cannot find module '" + spec + "'");
        err.code = info.errCode || "MODULE_NOT_FOUND";
        throw err;
      }
      if (info.kind === "builtin") {
        const mod = loadNative(info.path);
        if (mod !== undefined) return mod;
        throw moduleNotFound(spec);
      }
      return loadUserModule(info);
    }
    req.resolve = function (spec) {
      if (resolved.has(spec) || factories.has(spec)) return spec;
      const info = JSON.parse(__bento_loadModule(spec, parentPath || "", parentFormat || "commonjs"));
      if (!info.ok) throw moduleNotFound(spec);
      // Node's require.resolve answers a filename, so it answers in the operating
      // system's spelling and its result is a key into require.cache. A builtin
      // has no file and comes back as its own name.
      return info.filename || info.path;
    };
    req.cache = userCache;
    return req;
  }

  // The default require is bound to the current directory. The entry module and
  // each user module get their own directory-bound require through requireFrom.
  g.require = requireFrom("", "commonjs");

  // __bento_runEntry runs the transpiled entry file through the same module
  // wrapper as any other module, giving it its own record and a require bound to
  // its directory. The runtime calls this once per program.
  // The entry carries both spellings for the same reason a required module does:
  // path is the module path the resolver reads back as the referrer, filename is
  // the operating system's spelling the user's code sees.
  g.__bento_runEntry = function (path, code, filename, dir) {
    const record = {
      id: ".",
      filename: filename,
      exports: {},
      loaded: false,
      format: "commonjs",
    };
    userCache[filename] = record;
    const fn = compileModule(code, filename);
    // Keep the well-known globals pointing at the entry so scripts that read a
    // bare module or exports at top level still see the running module.
    g.module = record;
    g.exports = record.exports;
    fn.call(record.exports, record.exports, requireFrom(path, "commonjs"), record, filename, dir);
    record.loaded = true;
    g.exports = record.exports;
  };

  // A top-level module record for code that reads module/exports before the
  // entry runs. __bento_runEntry replaces it with the real entry record.
  g.module = { exports: {}, id: ".", loaded: false };
  g.exports = g.module.exports;

  // ---- native ES module interop -----------------------------------------
  // When a program runs as a real ES module, native imports of builtins, JSON,
  // data: URLs, and CommonJS files come back through __bento_esmShim. The Go
  // module host hands us the require specifier; we require it once, stash the
  // live exports in a slot the generated module reads, and return ES module
  // source that re-exports it. The default export is the whole exports object
  // and each own key that is a legal identifier becomes a named export, so both
  // `import x from` and `import { y } from` work against a CommonJS dependency.
  const esmSlots = Object.create(null);
  g.__bentoEsmSlots = esmSlots;

  // Reserved words cannot be named exports even though they are valid property
  // keys, so a dependency with a `default` or `class` key still exports the rest.
  const reservedWords = new Set([
    "default", "break", "case", "catch", "class", "const", "continue",
    "debugger", "delete", "do", "else", "enum", "export", "extends", "false",
    "finally", "for", "function", "if", "import", "in", "instanceof", "new",
    "null", "return", "super", "switch", "this", "throw", "true", "try",
    "typeof", "var", "void", "while", "with", "yield", "let", "await",
  ]);
  const identPattern = /^[A-Za-z_$][A-Za-z0-9_$]*$/;

  g.__bento_esmShim = function (spec) {
    const m = g.require(spec);
    esmSlots[spec] = m;
    const lit = JSON.stringify(spec);
    let src = "const __m = globalThis.__bentoEsmSlots[" + lit + "];\nexport default __m;\n";
    if (m && (typeof m === "object" || typeof m === "function")) {
      const names = [];
      const consider = (k) => {
        if (reservedWords.has(k) || names.indexOf(k) >= 0) return;
        if (!identPattern.test(k)) return;
        names.push(k);
      };
      Object.keys(m).forEach(consider);
      Object.getOwnPropertyNames(m).forEach(consider);
      for (const k of names) {
        src += "export const " + k + " = __m[" + JSON.stringify(k) + "];\n";
      }
    }
    return src;
  };

  // ---- structured clone / microtask helpers used by common libraries ----
  if (typeof g.structuredClone !== "function") {
    g.structuredClone = function (v) { return JSON.parse(JSON.stringify(v)); };
  }

  // ---- Error.captureStackTrace ------------------------------------------
  // V8's, and so Node's: it puts a stack on any object, not only on an Error.
  // Custom error classes call it in their constructor to hide the constructor
  // from the trace, and libraries that carry a stack on a plain result object
  // call it on that. Neither is in the language, so the engine does not have it.
  //
  // What it writes is V8's shape: a header line naming the object and its
  // message, then the frames, indented. QuickJS gives the frames alone with no
  // header, so the header is built here from the target rather than taken from
  // the captured error.
  if (typeof Error.captureStackTrace !== "function") {
    // V8 defaults to ten frames and lets a program raise or lower it; a limit of
    // zero asks for the header alone. It is a plain property, because code that
    // sets it expects the next capture to honor the new number.
    if (Error.stackTraceLimit === undefined) Error.stackTraceLimit = 10;

    Error.captureStackTrace = function (target, constructorOpt) {
      if (target === null || (typeof target !== "object" && typeof target !== "function")) {
        throw new TypeError("Error.captureStackTrace requires an object");
      }

      const limit = typeof Error.stackTraceLimit === "number" ? Math.max(0, Error.stackTraceLimit) : 10;
      const raw = new Error().stack || "";
      let frames = raw.split("\n").filter(function (line) { return line.trim() !== ""; });

      // The frame for this function is never part of the answer.
      frames = frames.slice(1);

      // constructorOpt asks for everything above and including that function to
      // be hidden, which is how a custom error class keeps its own constructor
      // out of the trace. QuickJS names frames but hands back no reference to
      // the function, so the match is by name; an anonymous one has nothing to
      // match on and is left alone rather than guessed at.
      const hideUpTo = constructorOpt && constructorOpt.name;
      if (hideUpTo) {
        for (let i = 0; i < frames.length; i++) {
          if (frames[i].indexOf(" " + hideUpTo + " ") !== -1) {
            frames = frames.slice(i + 1);
            break;
          }
        }
      }

      const name = target.name === undefined ? "Error" : String(target.name);
      const message = target.message === undefined || target.message === "" ? "" : String(target.message);
      const header = message === "" ? name : name + ": " + message;

      const stack = [header].concat(frames.slice(0, limit)).join("\n");
      // Writable and configurable but not enumerable, the way V8 installs it, so
      // an object that gets a stack does not start serializing one.
      Object.defineProperty(target, "stack", {
        value: stack,
        writable: true,
        enumerable: false,
        configurable: true,
      });
    };
  }
})();
