// buffer implements node:buffer. Buffer subclasses Uint8Array so it interops
// with typed-array APIs and the engine's ArrayBuffer machinery, and adds the
// Node encoding and read/write helpers on top.

__bento_defineModule("buffer", function (module, exports, require) {
  "use strict";

  const B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const B64URL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

  function base64Decode(str, urlSafe) {
    const table = urlSafe ? B64URL : B64;
    const lookup = base64Decode._lookup || (base64Decode._lookup = {});
    let map = lookup[table];
    if (!map) {
      map = lookup[table] = {};
      for (let i = 0; i < table.length; i++) map[table[i]] = i;
    }
    str = str.replace(/[^A-Za-z0-9+/=_-]/g, "");
    let end = str.length;
    while (end > 0 && str[end - 1] === "=") end--;
    const outLen = (end * 3) >> 2;
    const out = new Uint8Array(outLen);
    let bits = 0;
    let count = 0;
    let pos = 0;
    for (let i = 0; i < end; i++) {
      const v = map[str[i]];
      if (v === undefined) continue;
      bits = (bits << 6) | v;
      count += 6;
      if (count >= 8) {
        count -= 8;
        out[pos++] = (bits >> count) & 0xff;
      }
    }
    return out.subarray(0, pos);
  }

  function base64Encode(bytes, urlSafe) {
    const table = urlSafe ? B64URL : B64;
    let out = "";
    let i;
    for (i = 0; i + 2 < bytes.length; i += 3) {
      const n = (bytes[i] << 16) | (bytes[i + 1] << 8) | bytes[i + 2];
      out += table[(n >> 18) & 63] + table[(n >> 12) & 63] + table[(n >> 6) & 63] + table[n & 63];
    }
    const rem = bytes.length - i;
    if (rem === 1) {
      const n = bytes[i] << 16;
      out += table[(n >> 18) & 63] + table[(n >> 12) & 63];
      if (!urlSafe) out += "==";
    } else if (rem === 2) {
      const n = (bytes[i] << 16) | (bytes[i + 1] << 8);
      out += table[(n >> 18) & 63] + table[(n >> 12) & 63] + table[(n >> 6) & 63];
      if (!urlSafe) out += "=";
    }
    return out;
  }

  function hexDecode(str) {
    const clean = str.length % 2 ? str.slice(0, -1) : str;
    const out = new Uint8Array(clean.length >> 1);
    for (let i = 0; i < out.length; i++) {
      const byte = parseInt(clean.substr(i * 2, 2), 16);
      if (Number.isNaN(byte)) return out.subarray(0, i);
      out[i] = byte;
    }
    return out;
  }

  function hexEncode(bytes) {
    let out = "";
    for (let i = 0; i < bytes.length; i++) {
      out += (bytes[i] < 16 ? "0" : "") + bytes[i].toString(16);
    }
    return out;
  }

  function utf8Encode(str) {
    // TextEncoder is available in the engine; fall back to a manual encoder.
    if (typeof TextEncoder === "function") return new TextEncoder().encode(str);
    const bytes = [];
    for (let i = 0; i < str.length; i++) {
      let c = str.charCodeAt(i);
      if (c < 0x80) bytes.push(c);
      else if (c < 0x800) {
        bytes.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
      } else if (c >= 0xd800 && c <= 0xdbff) {
        const c2 = str.charCodeAt(++i);
        c = 0x10000 + ((c & 0x3ff) << 10) + (c2 & 0x3ff);
        bytes.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 0x3f), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
      } else {
        bytes.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
      }
    }
    return new Uint8Array(bytes);
  }

  function utf8Decode(bytes) {
    if (typeof TextDecoder === "function") return new TextDecoder("utf-8").decode(bytes);
    let out = "";
    for (let i = 0; i < bytes.length; ) {
      const b = bytes[i++];
      if (b < 0x80) out += String.fromCharCode(b);
      else if (b < 0xe0) out += String.fromCharCode(((b & 0x1f) << 6) | (bytes[i++] & 0x3f));
      else if (b < 0xf0) out += String.fromCharCode(((b & 0x0f) << 12) | ((bytes[i++] & 0x3f) << 6) | (bytes[i++] & 0x3f));
      else {
        const cp = ((b & 0x07) << 18) | ((bytes[i++] & 0x3f) << 12) | ((bytes[i++] & 0x3f) << 6) | (bytes[i++] & 0x3f);
        const off = cp - 0x10000;
        out += String.fromCharCode(0xd800 + (off >> 10), 0xdc00 + (off & 0x3ff));
      }
    }
    return out;
  }

  function normalizeEncoding(enc) {
    if (!enc) return "utf8";
    enc = ("" + enc).toLowerCase();
    if (enc === "utf-8") return "utf8";
    if (enc === "ucs2" || enc === "ucs-2") return "utf16le";
    if (enc === "binary") return "latin1";
    return enc;
  }

  function bytesFromString(str, encoding) {
    encoding = normalizeEncoding(encoding);
    switch (encoding) {
      case "utf8": return utf8Encode(str);
      case "hex": return hexDecode(str);
      case "base64": return base64Decode(str, false);
      case "base64url": return base64Decode(str, true);
      case "ascii":
      case "latin1": {
        const out = new Uint8Array(str.length);
        for (let i = 0; i < str.length; i++) out[i] = str.charCodeAt(i) & 0xff;
        return out;
      }
      case "utf16le": {
        const out = new Uint8Array(str.length * 2);
        for (let i = 0; i < str.length; i++) {
          const c = str.charCodeAt(i);
          out[i * 2] = c & 0xff;
          out[i * 2 + 1] = c >> 8;
        }
        return out;
      }
      default:
        throw new TypeError("Unknown encoding: " + encoding);
    }
  }

  function bytesToString(bytes, encoding) {
    encoding = normalizeEncoding(encoding);
    switch (encoding) {
      case "utf8": return utf8Decode(bytes);
      case "hex": return hexEncode(bytes);
      case "base64": return base64Encode(bytes, false);
      case "base64url": return base64Encode(bytes, true);
      case "ascii": {
        let out = "";
        for (let i = 0; i < bytes.length; i++) out += String.fromCharCode(bytes[i] & 0x7f);
        return out;
      }
      case "latin1": {
        let out = "";
        for (let i = 0; i < bytes.length; i++) out += String.fromCharCode(bytes[i]);
        return out;
      }
      case "utf16le": {
        let out = "";
        for (let i = 0; i + 1 < bytes.length; i += 2) out += String.fromCharCode(bytes[i] | (bytes[i + 1] << 8));
        return out;
      }
      default:
        throw new TypeError("Unknown encoding: " + encoding);
    }
  }

  class Buffer extends Uint8Array {
    static alloc(size, fill, encoding) {
      const buf = new Buffer(size);
      if (fill !== undefined && fill !== 0) buf.fill(fill, 0, size, encoding);
      return buf;
    }
    static allocUnsafe(size) {
      return new Buffer(size);
    }
    static allocUnsafeSlow(size) {
      return new Buffer(size);
    }
    static from(value, encodingOrOffset, length) {
      if (typeof value === "string") {
        return toBuffer(bytesFromString(value, encodingOrOffset));
      }
      if (value instanceof ArrayBuffer) {
        const view = length === undefined
          ? new Uint8Array(value, encodingOrOffset || 0)
          : new Uint8Array(value, encodingOrOffset || 0, length);
        return toBuffer(view.slice());
      }
      if (ArrayBuffer.isView(value)) {
        return toBuffer(new Uint8Array(value.buffer, value.byteOffset, value.byteLength).slice());
      }
      if (Array.isArray(value) || (value && typeof value.length === "number")) {
        return toBuffer(Uint8Array.from(value));
      }
      throw new TypeError("First argument must be a string, Buffer, ArrayBuffer, Array, or array-like object.");
    }
    static concat(list, totalLength) {
      if (totalLength === undefined) {
        totalLength = 0;
        for (let i = 0; i < list.length; i++) totalLength += list[i].length;
      }
      const out = new Buffer(totalLength);
      let pos = 0;
      for (let i = 0; i < list.length && pos < totalLength; i++) {
        const item = list[i];
        const take = Math.min(item.length, totalLength - pos);
        out.set(item.subarray(0, take), pos);
        pos += take;
      }
      return out;
    }
    static isBuffer(x) {
      return x instanceof Buffer;
    }
    static isEncoding(enc) {
      try { normalizeEncoding(enc); return ["utf8", "hex", "base64", "base64url", "ascii", "latin1", "utf16le"].indexOf(normalizeEncoding(enc)) !== -1; }
      catch (e) { return false; }
    }
    static byteLength(value, encoding) {
      if (typeof value !== "string") return value.byteLength !== undefined ? value.byteLength : value.length;
      return bytesFromString(value, encoding).length;
    }

    toString(encoding, start, end) {
      start = start || 0;
      end = end === undefined ? this.length : end;
      return bytesToString(this.subarray(start, end), encoding);
    }
    toJSON() {
      return { type: "Buffer", data: Array.prototype.slice.call(this) };
    }
    equals(other) {
      if (this.length !== other.length) return false;
      for (let i = 0; i < this.length; i++) if (this[i] !== other[i]) return false;
      return true;
    }
    compare(other) {
      const len = Math.min(this.length, other.length);
      for (let i = 0; i < len; i++) {
        if (this[i] < other[i]) return -1;
        if (this[i] > other[i]) return 1;
      }
      if (this.length < other.length) return -1;
      if (this.length > other.length) return 1;
      return 0;
    }
    write(string, offset, length, encoding) {
      if (typeof offset === "string") { encoding = offset; offset = 0; length = this.length; }
      else if (typeof length === "string") { encoding = length; length = this.length - offset; }
      offset = offset || 0;
      const bytes = bytesFromString(string, encoding);
      const take = Math.min(bytes.length, length === undefined ? this.length - offset : length, this.length - offset);
      this.set(bytes.subarray(0, take), offset);
      return take;
    }
    slice(start, end) {
      return toBuffer(this.subarray(start, end));
    }
    fill(value, start, end, encoding) {
      start = start || 0;
      end = end === undefined ? this.length : end;
      if (typeof value === "string") {
        const bytes = bytesFromString(value, encoding);
        if (bytes.length === 0) return this;
        for (let i = start; i < end; i++) this[i] = bytes[(i - start) % bytes.length];
        return this;
      }
      for (let i = start; i < end; i++) this[i] = value & 0xff;
      return this;
    }
    readUInt8(o) { return this[o]; }
    readInt8(o) { const v = this[o]; return v & 0x80 ? v - 0x100 : v; }
    readUInt16LE(o) { return this[o] | (this[o + 1] << 8); }
    readUInt16BE(o) { return (this[o] << 8) | this[o + 1]; }
    readUInt32LE(o) { return (this[o] | (this[o + 1] << 8) | (this[o + 2] << 16)) + this[o + 3] * 0x1000000; }
    readUInt32BE(o) { return this[o] * 0x1000000 + ((this[o + 1] << 16) | (this[o + 2] << 8) | this[o + 3]); }
    readInt32LE(o) { return this[o] | (this[o + 1] << 8) | (this[o + 2] << 16) | (this[o + 3] << 24); }
    readInt32BE(o) { return (this[o] << 24) | (this[o + 1] << 16) | (this[o + 2] << 8) | this[o + 3]; }
    writeUInt8(v, o) { this[o] = v & 0xff; return o + 1; }
    writeInt8(v, o) { this[o] = v & 0xff; return o + 1; }
    writeUInt16LE(v, o) { this[o] = v & 0xff; this[o + 1] = (v >> 8) & 0xff; return o + 2; }
    writeUInt16BE(v, o) { this[o] = (v >> 8) & 0xff; this[o + 1] = v & 0xff; return o + 2; }
    writeUInt32LE(v, o) { this[o] = v & 0xff; this[o + 1] = (v >> 8) & 0xff; this[o + 2] = (v >> 16) & 0xff; this[o + 3] = (v >> 24) & 0xff; return o + 4; }
    writeUInt32BE(v, o) { this[o] = (v >> 24) & 0xff; this[o + 1] = (v >> 16) & 0xff; this[o + 2] = (v >> 8) & 0xff; this[o + 3] = v & 0xff; return o + 4; }
  }

  function toBuffer(u8) {
    Object.setPrototypeOf(u8, Buffer.prototype);
    return u8;
  }

  module.exports = {
    Buffer: Buffer,
    SlowBuffer: function (n) { return Buffer.alloc(n); },
    kMaxLength: 0x7fffffff,
    constants: { MAX_LENGTH: 0x7fffffff, MAX_STRING_LENGTH: 0x1fffffff },
    atob: function (s) { return bytesToString(base64Decode(s, false), "latin1"); },
    btoa: function (s) { return base64Encode(bytesFromString(s, "latin1"), false); },
    _bytesFromString: bytesFromString,
    _bytesToString: bytesToString,
  };
});
