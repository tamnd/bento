// fs implements node:fs. The Go host exposes a small set of synchronous file
// primitives that return a JSON envelope ({ok, ...} or {ok:false, code, msg})
// and carry binary payloads as base64. This module builds the Node surface on
// top: the *Sync calls, the callback forms, and the fs.promises namespace.

__bento_defineModule("fs", function (module, exports, require) {
  "use strict";

  const { Buffer } = require("buffer");
  const path = require("path");

  function makeError(env, syscall, pathArg) {
    const err = new Error(
      (env.code || "Error") + ": " + (env.msg || "operation failed") +
      (pathArg ? ", " + syscall + " '" + pathArg + "'" : "")
    );
    err.code = env.code || "UNKNOWN";
    err.errno = env.errno || -1;
    err.syscall = syscall;
    if (pathArg) err.path = pathArg;
    return err;
  }

  function call(name, args, syscall, pathArg) {
    const env = JSON.parse(name.apply(null, args));
    if (!env.ok) throw makeError(env, syscall, pathArg);
    return env;
  }

  function decode(env, encoding) {
    const buf = Buffer.from(env.b64 || "", "base64");
    if (!encoding || encoding === "buffer" || (encoding && encoding.encoding === "buffer")) {
      if (encoding && typeof encoding === "object" && encoding.encoding && encoding.encoding !== "buffer") {
        return buf.toString(encoding.encoding);
      }
      return buf;
    }
    const enc = typeof encoding === "string" ? encoding : encoding.encoding;
    return enc ? buf.toString(enc) : buf;
  }

  function toBuffer(data, encoding) {
    if (Buffer.isBuffer(data)) return data;
    if (data instanceof Uint8Array) return Buffer.from(data);
    if (typeof data === "string") return Buffer.from(data, encoding && encoding.encoding ? encoding.encoding : encoding || "utf8");
    return Buffer.from(String(data), "utf8");
  }

  function makeStats(s) {
    const stat = {
      dev: s.dev, ino: s.ino, mode: s.mode, nlink: s.nlink, uid: s.uid, gid: s.gid,
      rdev: s.rdev || 0, size: s.size, blksize: s.blksize || 4096, blocks: s.blocks || 0,
      atimeMs: s.atimeMs, mtimeMs: s.mtimeMs, ctimeMs: s.ctimeMs, birthtimeMs: s.birthtimeMs || s.ctimeMs,
      atime: new Date(s.atimeMs), mtime: new Date(s.mtimeMs), ctime: new Date(s.ctimeMs),
      birthtime: new Date(s.birthtimeMs || s.ctimeMs),
      isFile: () => s.kind === "file",
      isDirectory: () => s.kind === "dir",
      isSymbolicLink: () => s.kind === "symlink",
      isBlockDevice: () => false,
      isCharacterDevice: () => false,
      isFIFO: () => false,
      isSocket: () => false,
    };
    return stat;
  }

  // ---- synchronous API --------------------------------------------------
  function readFileSync(p, options) {
    const env = call(__bento_fs_read, [String(p)], "open", p);
    return decode(env, options);
  }
  function writeFileSync(p, data, options) {
    const buf = toBuffer(data, options);
    call(__bento_fs_write, [String(p), buf.toString("base64"), "w"], "open", p);
  }
  function appendFileSync(p, data, options) {
    const buf = toBuffer(data, options);
    call(__bento_fs_write, [String(p), buf.toString("base64"), "a"], "open", p);
  }
  function existsSync(p) {
    try { const env = JSON.parse(__bento_fs_stat(String(p))); return !!env.ok; }
    catch (e) { return false; }
  }
  function statSync(p) {
    return makeStats(call(__bento_fs_stat, [String(p)], "stat", p).stat);
  }
  function lstatSync(p) {
    return makeStats(call(__bento_fs_lstat, [String(p)], "lstat", p).stat);
  }
  function mkdirSync(p, options) {
    const recursive = options === true || (options && options.recursive);
    call(__bento_fs_mkdir, [String(p), !!recursive], "mkdir", p);
  }
  function rmdirSync(p, options) {
    const recursive = options && options.recursive;
    call(__bento_fs_rm, [String(p), !!recursive, false], "rmdir", p);
  }
  function rmSync(p, options) {
    const recursive = options && options.recursive;
    const force = options && options.force;
    try { call(__bento_fs_rm, [String(p), !!recursive, false], "unlink", p); }
    catch (e) { if (!force) throw e; }
  }
  function unlinkSync(p) {
    call(__bento_fs_rm, [String(p), false, false], "unlink", p);
  }
  function readdirSync(p, options) {
    const env = call(__bento_fs_readdir, [String(p)], "scandir", p);
    const withTypes = options && options.withFileTypes;
    if (withTypes) {
      return env.entries.map((e) => new Dirent(e.name, e.kind));
    }
    return env.entries.map((e) => e.name);
  }
  function renameSync(from, to) {
    call(__bento_fs_rename, [String(from), String(to)], "rename", from);
  }
  function copyFileSync(from, to) {
    call(__bento_fs_copy, [String(from), String(to)], "copyfile", from);
  }
  function realpathSync(p) {
    return call(__bento_fs_realpath, [String(p)], "realpath", p).path;
  }
  function readlinkSync(p) {
    return call(__bento_fs_readlink, [String(p)], "readlink", p).path;
  }
  function symlinkSync(target, p) {
    call(__bento_fs_symlink, [String(target), String(p)], "symlink", p);
  }
  function chmodSync(p, mode) {
    call(__bento_fs_chmod, [String(p), mode | 0], "chmod", p);
  }
  function mkdtempSync(prefix) {
    return call(__bento_fs_mkdtemp, [String(prefix)], "mkdtemp", prefix).path;
  }

  class Dirent {
    constructor(name, kind) { this.name = name; this._kind = kind; }
    isFile() { return this._kind === "file"; }
    isDirectory() { return this._kind === "dir"; }
    isSymbolicLink() { return this._kind === "symlink"; }
    isBlockDevice() { return false; }
    isCharacterDevice() { return false; }
    isFIFO() { return false; }
    isSocket() { return false; }
  }

  // ---- callback API -----------------------------------------------------
  // Each async call runs its synchronous core on a microtask and dispatches the
  // node-style (err, result) callback. True nonblocking I/O lands in a later
  // milestone; the observable contract (async delivery, error-first) holds now.
  function asyncify(syncFn) {
    return function () {
      const args = Array.prototype.slice.call(arguments);
      const cb = args.pop();
      if (typeof cb !== "function") throw new TypeError("callback must be a function");
      Promise.resolve().then(() => {
        let result, error;
        try { result = syncFn.apply(null, args); }
        catch (e) { error = e; }
        if (error) cb(error);
        else if (result === undefined) cb(null);
        else cb(null, result);
      });
    };
  }

  const promises = {};
  function promisifySync(syncFn) {
    return function () {
      const args = arguments;
      return new Promise((resolve, reject) => {
        Promise.resolve().then(() => {
          try { resolve(syncFn.apply(null, args)); }
          catch (e) { reject(e); }
        });
      });
    };
  }

  const api = {
    readFileSync: readFileSync,
    writeFileSync: writeFileSync,
    appendFileSync: appendFileSync,
    existsSync: existsSync,
    statSync: statSync,
    lstatSync: lstatSync,
    mkdirSync: mkdirSync,
    rmdirSync: rmdirSync,
    rmSync: rmSync,
    unlinkSync: unlinkSync,
    readdirSync: readdirSync,
    renameSync: renameSync,
    copyFileSync: copyFileSync,
    realpathSync: realpathSync,
    readlinkSync: readlinkSync,
    symlinkSync: symlinkSync,
    chmodSync: chmodSync,
    mkdtempSync: mkdtempSync,
    Dirent: Dirent,
    constants: {
      F_OK: 0, R_OK: 4, W_OK: 2, X_OK: 1,
      O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_EXCL: 128, O_TRUNC: 512, O_APPEND: 1024,
    },
    accessSync: function (p) { statSync(p); },
    access: undefined,
  };

  // Wire the callback and promise forms for the file operations.
  const asyncNames = {
    readFile: readFileSync, writeFile: writeFileSync, appendFile: appendFileSync,
    stat: statSync, lstat: lstatSync, mkdir: mkdirSync, rmdir: rmdirSync, rm: rmSync,
    unlink: unlinkSync, readdir: readdirSync, rename: renameSync, copyFile: copyFileSync,
    realpath: realpathSync, readlink: readlinkSync, symlink: symlinkSync, chmod: chmodSync,
    mkdtemp: mkdtempSync,
  };
  for (const name of Object.keys(asyncNames)) {
    api[name] = asyncify(asyncNames[name]);
    promises[name] = promisifySync(asyncNames[name]);
  }
  api.access = function (p, mode, cb) {
    if (typeof mode === "function") { cb = mode; }
    asyncify(statSync)(p, cb);
  };
  promises.access = promisifySync(statSync);

  api.promises = promises;
  module.exports = api;
});
