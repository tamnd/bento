// path implements node:path with both posix and win32 variants.
//
// The default export is the variant matching the host platform, exposed through
// process.platform. Both variants are always reachable as path.posix and
// path.win32 so cross-platform tooling can pick one explicitly.

__bento_defineModule("path", function (module, exports, require) {
  "use strict";

  function assertString(name, value) {
    if (typeof value !== "string") {
      throw new TypeError(
        "The \"" + name + "\" argument must be of type string. Received " + typeof value
      );
    }
  }

  function makePath(sep, delimiter, isWin) {
    const other = isWin ? "/" : "\\";

    function isSep(c) {
      return c === sep || c === other;
    }

    function normalizeArray(parts, allowAboveRoot) {
      const res = [];
      for (let i = 0; i < parts.length; i++) {
        const p = parts[i];
        if (!p || p === ".") continue;
        if (p === "..") {
          if (res.length && res[res.length - 1] !== "..") res.pop();
          else if (allowAboveRoot) res.push("..");
        } else {
          res.push(p);
        }
      }
      return res;
    }

    function normalize(path) {
      assertString("path", path);
      if (path.length === 0) return ".";
      const isAbsolute = isSep(path.charCodeAt(0) ? path[0] : "");
      const abs = isSep(path[0]);
      const trailing = isSep(path[path.length - 1]);
      let segments = path.split(/[\\/]+/);
      segments = normalizeArray(segments, !abs);
      let out = segments.join(sep);
      if (!out && !abs) out = ".";
      if (out && trailing) out += sep;
      if (abs) out = sep + out;
      return out;
    }

    function join() {
      const args = Array.prototype.slice.call(arguments);
      if (args.length === 0) return ".";
      let joined = "";
      for (let i = 0; i < args.length; i++) {
        assertString("path", args[i]);
        if (args[i].length === 0) continue;
        joined = joined ? joined + sep + args[i] : args[i];
      }
      if (!joined) return ".";
      return normalize(joined);
    }

    function isAbsolute(path) {
      assertString("path", path);
      if (path.length === 0) return false;
      if (isWin) {
        if (isSep(path[0])) return true;
        return /^[a-zA-Z]:[\\/]/.test(path);
      }
      return path[0] === "/";
    }

    function resolve() {
      let resolved = "";
      let resolvedAbsolute = false;
      for (let i = arguments.length - 1; i >= -1 && !resolvedAbsolute; i--) {
        const path = i >= 0 ? arguments[i] : (typeof process !== "undefined" ? process.cwd() : "/");
        assertString("path", path);
        if (path.length === 0) continue;
        resolved = path + sep + resolved;
        resolvedAbsolute = isAbsolute(path);
      }
      const parts = normalizeArray(resolved.split(/[\\/]+/), !resolvedAbsolute);
      let out = parts.join(sep);
      if (resolvedAbsolute) return sep + out;
      return out.length ? out : ".";
    }

    function dirname(path) {
      assertString("path", path);
      if (path.length === 0) return ".";
      let end = -1;
      let sawNonSep = false;
      for (let i = path.length - 1; i >= 1; i--) {
        if (isSep(path[i])) {
          if (sawNonSep) { end = i; break; }
        } else {
          sawNonSep = true;
        }
      }
      if (end === -1) return isSep(path[0]) ? sep : ".";
      if (end === 0) return sep;
      return path.slice(0, end);
    }

    function basename(path, ext) {
      assertString("path", path);
      const segs = path.split(/[\\/]+/).filter(Boolean);
      let base = segs.length ? segs[segs.length - 1] : "";
      if (ext && base.endsWith(ext) && base !== ext) {
        base = base.slice(0, base.length - ext.length);
      }
      return base;
    }

    function extname(path) {
      assertString("path", path);
      const base = basename(path);
      const dot = base.lastIndexOf(".");
      if (dot <= 0) return "";
      return base.slice(dot);
    }

    function parse(path) {
      assertString("path", path);
      const root = isAbsolute(path) ? sep : "";
      const dir = dirname(path);
      const base = basename(path);
      const ext = extname(path);
      const name = ext ? base.slice(0, base.length - ext.length) : base;
      return { root: root, dir: path.length && dir !== "." ? dir : (root || ""), base: base, ext: ext, name: name };
    }

    function format(obj) {
      const dir = obj.dir || obj.root || "";
      const base = obj.base || ((obj.name || "") + (obj.ext || ""));
      if (!dir) return base;
      if (dir === obj.root) return dir + base;
      return dir + sep + base;
    }

    function relative(from, to) {
      assertString("from", from);
      assertString("to", to);
      from = resolve(from);
      to = resolve(to);
      if (from === to) return "";
      const fromParts = from.split(/[\\/]+/).filter(Boolean);
      const toParts = to.split(/[\\/]+/).filter(Boolean);
      let i = 0;
      while (i < fromParts.length && i < toParts.length && fromParts[i] === toParts[i]) i++;
      const up = [];
      for (let j = i; j < fromParts.length; j++) up.push("..");
      return up.concat(toParts.slice(i)).join(sep);
    }

    function toNamespacedPath(path) {
      return path;
    }

    const api = {
      sep: sep,
      delimiter: delimiter,
      normalize: normalize,
      join: join,
      resolve: resolve,
      isAbsolute: isAbsolute,
      dirname: dirname,
      basename: basename,
      extname: extname,
      parse: parse,
      format: format,
      relative: relative,
      toNamespacedPath: toNamespacedPath,
    };
    return api;
  }

  const posix = makePath("/", ":", false);
  const win32 = makePath("\\", ";", true);
  posix.posix = posix;
  posix.win32 = win32;
  win32.posix = posix;
  win32.win32 = win32;

  const isWindows = typeof process !== "undefined" && process.platform === "win32";
  module.exports = isWindows ? win32 : posix;
});
