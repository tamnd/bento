// zlib implements node:zlib on top of one Go host call that runs a codec end to
// end and carries both sides as base64. It covers the three framings the Go
// standard library implements, zlib (deflate), raw deflate and gzip, in all
// three shapes Node offers them: the *Sync calls, the callback forms, and the
// Transform streams.
//
// Two things Node has are not here. Brotli would need a compressor outside the
// standard library, and bento links no C, so brotliCompress and its siblings are
// absent rather than present and throwing: code that feature-detects them then
// takes its non-brotli path. And the streams buffer their whole input and run
// the codec once at flush, rather than compressing incrementally, so a stream
// over a large file holds it in memory and emits one chunk. What comes out is
// the same bytes either way; what differs is when, and how much is resident.

__bento_defineModule("zlib", function (module, exports, require) {
  "use strict";

  const { Buffer } = require("buffer");
  const { Transform } = require("stream");

  // The subset of zlib's constants a caller actually passes or compares
  // against. Node exposes these both on the module and under .constants, and
  // programs read them from either place.
  const constants = {
    Z_NO_FLUSH: 0,
    Z_PARTIAL_FLUSH: 1,
    Z_SYNC_FLUSH: 2,
    Z_FULL_FLUSH: 3,
    Z_FINISH: 4,
    Z_BLOCK: 5,
    Z_TREES: 6,

    Z_OK: 0,
    Z_STREAM_END: 1,
    Z_NEED_DICT: 2,
    Z_ERRNO: -1,
    Z_STREAM_ERROR: -2,
    Z_DATA_ERROR: -3,
    Z_MEM_ERROR: -4,
    Z_BUF_ERROR: -5,
    Z_VERSION_ERROR: -6,

    Z_NO_COMPRESSION: 0,
    Z_BEST_SPEED: 1,
    Z_BEST_COMPRESSION: 9,
    Z_DEFAULT_COMPRESSION: -1,

    Z_FILTERED: 1,
    Z_HUFFMAN_ONLY: 2,
    Z_RLE: 3,
    Z_FIXED: 4,
    Z_DEFAULT_STRATEGY: 0,

    DEFLATE: 1,
    INFLATE: 2,
    GZIP: 3,
    GUNZIP: 4,
    DEFLATERAW: 5,
    INFLATERAW: 6,
    UNZIP: 7,
  };

  // The numbers zlib returns for the codes the host can report, so err.errno
  // matches err.code the way it does in Node.
  const ERRNO = {
    Z_DATA_ERROR: constants.Z_DATA_ERROR,
    Z_BUF_ERROR: constants.Z_BUF_ERROR,
    Z_STREAM_ERROR: constants.Z_STREAM_ERROR,
  };

  // Node accepts a string, a Buffer, an ArrayBuffer or any view over one. A
  // string is utf8, which is what Buffer.from defaults to.
  function toBuffer(data) {
    if (Buffer.isBuffer(data)) return data;
    if (typeof data === "string") return Buffer.from(data, "utf8");
    if (data instanceof ArrayBuffer) return Buffer.from(new Uint8Array(data));
    if (ArrayBuffer.isView(data)) return Buffer.from(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
    throw new TypeError("The \"buffer\" argument must be of type string or an instance of Buffer, TypedArray, DataView, or ArrayBuffer");
  }

  function levelOf(opts) {
    if (opts && typeof opts.level === "number") return opts.level;
    return constants.Z_DEFAULT_COMPRESSION;
  }

  function run(format, direction, data, opts) {
    const input = toBuffer(data);
    const env = JSON.parse(__bento_zlib_run(format, direction, input.toString("base64"), levelOf(opts)));
    if (!env.ok) {
      const err = new Error(env.msg || "zlib error");
      err.code = env.code || "Z_DATA_ERROR";
      err.errno = ERRNO[err.code] !== undefined ? ERRNO[err.code] : constants.Z_ERRNO;
      throw err;
    }
    return Buffer.from(env.b64 || "", "base64");
  }

  // The callback forms take (buffer, [options], callback) and, like Node's, are
  // asynchronous even though the work underneath is not: a caller that passes a
  // callback expects it after the current turn, not during the call.
  function async(format, direction) {
    return function (data, opts, cb) {
      if (typeof opts === "function") {
        cb = opts;
        opts = undefined;
      }
      if (typeof cb !== "function") throw new TypeError("Callback must be a function");
      let result, error;
      try {
        result = run(format, direction, data, opts);
      } catch (e) {
        error = e;
      }
      Promise.resolve().then(function () {
        if (error) cb(error);
        else cb(null, result);
      });
    };
  }

  // Zlib is the Transform every codec stream is. It collects the chunks written
  // to it and runs the codec once, on flush: see the note at the top of the file
  // for what that costs and what it does not change.
  class Zlib extends Transform {
    constructor(format, direction, opts) {
      super(opts);
      this._format = format;
      this._direction = direction;
      this._opts = opts || {};
      this._chunks = [];
      this.bytesWritten = 0;
    }
    _transform(chunk, encoding, callback) {
      const buf = toBuffer(chunk);
      this.bytesWritten += buf.length;
      this._chunks.push(buf);
      callback();
    }
    _flush(callback) {
      let out;
      try {
        out = run(this._format, this._direction, Buffer.concat(this._chunks), this._opts);
      } catch (e) {
        callback(e);
        return;
      }
      this.push(out);
      callback();
    }
  }

  // Each codec gets the four names Node gives it: the class, its factory, the
  // synchronous call and the callback form.
  const CODECS = [
    { name: "Deflate", fn: "deflate", format: "deflate", direction: "compress" },
    { name: "Inflate", fn: "inflate", format: "deflate", direction: "decompress" },
    { name: "DeflateRaw", fn: "deflateRaw", format: "deflateRaw", direction: "compress" },
    { name: "InflateRaw", fn: "inflateRaw", format: "deflateRaw", direction: "decompress" },
    { name: "Gzip", fn: "gzip", format: "gzip", direction: "compress" },
    { name: "Gunzip", fn: "gunzip", format: "gzip", direction: "decompress" },
    // unzip is not a fourth framing but inflate's sniffing mode: it takes either
    // a gzip member or a zlib stream and decides by the header.
    { name: "Unzip", fn: "unzip", format: "unzip", direction: "decompress" },
  ];

  const zlib = { constants: constants };
  for (const key of Object.keys(constants)) zlib[key] = constants[key];

  for (const codec of CODECS) {
    const cls = class extends Zlib {
      constructor(opts) {
        super(codec.format, codec.direction, opts);
      }
    };
    Object.defineProperty(cls, "name", { value: codec.name });
    zlib[codec.name] = cls;
    zlib["create" + codec.name] = (opts) => new cls(opts);
    zlib[codec.fn + "Sync"] = (data, opts) => run(codec.format, codec.direction, data, opts);
    zlib[codec.fn] = async(codec.format, codec.direction);
  }

  module.exports = zlib;
});
