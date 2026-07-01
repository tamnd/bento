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
  // A minimal registry so require() has a home. The node core layer and the
  // real resolver land in later milestones; for now this gives a clear error
  // for anything not yet built in and lets code that never imports run cleanly.
  const builtins = new Map();
  g.__bento_registerModule = function (name, exportsObj) {
    builtins.set(name, exportsObj);
    if (name.indexOf("node:") !== 0) builtins.set("node:" + name, exportsObj);
  };
  g.require = function (spec) {
    if (builtins.has(spec)) return builtins.get(spec);
    const err = new Error("Cannot find module '" + spec + "'");
    err.code = "MODULE_NOT_FOUND";
    throw err;
  };
  g.require.resolve = function (spec) {
    if (builtins.has(spec)) return spec;
    const err = new Error("Cannot find module '" + spec + "'");
    err.code = "MODULE_NOT_FOUND";
    throw err;
  };

  // A single top-level module record for the entry file. The real module system
  // gives each file its own record; this is enough to run one entry point.
  g.module = { exports: {}, id: ".", loaded: false };
  g.exports = g.module.exports;

  // ---- structured clone / microtask helpers used by common libraries ----
  if (typeof g.structuredClone !== "function") {
    g.structuredClone = function (v) { return JSON.parse(JSON.stringify(v)); };
  }
})();
