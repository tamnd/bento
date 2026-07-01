// stream implements a practical subset of node:stream: Readable, Writable,
// Duplex, Transform, and PassThrough built on EventEmitter. It covers the
// object-mode and buffered flowing paths that most libraries drive, without the
// full backpressure state machine of Node core.

__bento_defineModule("stream", function (module, exports, require) {
  "use strict";

  const EventEmitter = require("events");

  function Stream() {
    EventEmitter.call(this);
  }
  Object.setPrototypeOf(Stream.prototype, EventEmitter.prototype);

  class Readable extends EventEmitter {
    constructor(opts) {
      super();
      opts = opts || {};
      this._readableState = { flowing: null, ended: false, buffer: [], objectMode: !!opts.objectMode };
      if (typeof opts.read === "function") this._read = opts.read;
      this.readable = true;
    }
    _read() {}
    push(chunk) {
      const state = this._readableState;
      if (chunk === null) {
        state.ended = true;
        this.emit("end");
        return false;
      }
      if (state.flowing) {
        this.emit("data", chunk);
      } else {
        state.buffer.push(chunk);
        this.emit("readable");
      }
      return true;
    }
    read() {
      const state = this._readableState;
      if (state.buffer.length) return state.buffer.shift();
      return null;
    }
    on(event, listener) {
      const r = super.on(event, listener);
      if (event === "data") this.resume();
      return r;
    }
    resume() {
      const state = this._readableState;
      if (state.flowing) return this;
      state.flowing = true;
      while (state.buffer.length) this.emit("data", state.buffer.shift());
      if (state.ended) this.emit("end");
      this._read();
      return this;
    }
    pause() {
      this._readableState.flowing = false;
      return this;
    }
    pipe(dest) {
      this.on("data", (chunk) => dest.write(chunk));
      this.on("end", () => dest.end());
      dest.emit("pipe", this);
      return dest;
    }
    [Symbol.asyncIterator]() {
      const self = this;
      const queue = [];
      let done = false;
      let waiting = null;
      self.on("data", (c) => {
        if (waiting) { const w = waiting; waiting = null; w({ value: c, done: false }); }
        else queue.push(c);
      });
      self.on("end", () => {
        done = true;
        if (waiting) { const w = waiting; waiting = null; w({ value: undefined, done: true }); }
      });
      return {
        next() {
          if (queue.length) return Promise.resolve({ value: queue.shift(), done: false });
          if (done) return Promise.resolve({ value: undefined, done: true });
          return new Promise((resolve) => { waiting = resolve; });
        },
        [Symbol.asyncIterator]() { return this; },
      };
    }
    static from(iterable) {
      const r = new Readable({ objectMode: true });
      Promise.resolve().then(async () => {
        for await (const chunk of iterable) r.push(chunk);
        r.push(null);
      });
      return r;
    }
  }

  class Writable extends EventEmitter {
    constructor(opts) {
      super();
      opts = opts || {};
      this._writableState = { ended: false, objectMode: !!opts.objectMode };
      if (typeof opts.write === "function") this._write = opts.write;
      if (typeof opts.final === "function") this._final = opts.final;
      this.writable = true;
    }
    _write(chunk, encoding, callback) { callback(); }
    _final(callback) { callback(); }
    write(chunk, encoding, callback) {
      if (typeof encoding === "function") { callback = encoding; encoding = undefined; }
      this._write(chunk, encoding, (err) => {
        if (err) this.emit("error", err);
        else if (callback) callback();
      });
      return true;
    }
    end(chunk, encoding, callback) {
      if (typeof chunk === "function") { callback = chunk; chunk = undefined; }
      else if (typeof encoding === "function") { callback = encoding; encoding = undefined; }
      const finish = () => {
        this._final(() => {
          this._writableState.ended = true;
          this.emit("finish");
          this.emit("close");
          if (callback) callback();
        });
      };
      if (chunk !== undefined && chunk !== null) this.write(chunk, encoding, finish);
      else finish();
      return this;
    }
  }

  class Duplex extends Readable {
    constructor(opts) {
      super(opts);
      opts = opts || {};
      this._writableState = { ended: false, objectMode: !!opts.objectMode };
      if (typeof opts.write === "function") this._write = opts.write;
      this.writable = true;
    }
  }
  Duplex.prototype._write = Writable.prototype._write;
  Duplex.prototype._final = Writable.prototype._final;
  Duplex.prototype.write = Writable.prototype.write;
  Duplex.prototype.end = Writable.prototype.end;

  class Transform extends Duplex {
    constructor(opts) {
      super(opts);
      opts = opts || {};
      if (typeof opts.transform === "function") this._transform = opts.transform;
      if (typeof opts.flush === "function") this._flush = opts.flush;
    }
    _transform(chunk, encoding, callback) { callback(null, chunk); }
    _flush(callback) { callback(); }
    _write(chunk, encoding, callback) {
      this._transform(chunk, encoding, (err, data) => {
        if (err) return callback(err);
        if (data !== undefined && data !== null) this.push(data);
        callback();
      });
    }
    _final(callback) {
      this._flush((err, data) => {
        if (data !== undefined && data !== null) this.push(data);
        this.push(null);
        callback(err);
      });
    }
  }

  class PassThrough extends Transform {
    _transform(chunk, encoding, callback) { callback(null, chunk); }
  }

  function finished(stream, callback) {
    let called = false;
    const done = (err) => { if (!called) { called = true; callback(err); } };
    stream.on("end", () => done());
    stream.on("finish", () => done());
    stream.on("close", () => done());
    stream.on("error", (err) => done(err));
  }

  function pipeline() {
    const args = Array.prototype.slice.call(arguments);
    const callback = typeof args[args.length - 1] === "function" ? args.pop() : null;
    for (let i = 0; i < args.length - 1; i++) args[i].pipe(args[i + 1]);
    const last = args[args.length - 1];
    if (callback) finished(last, callback);
    return last;
  }

  Stream.Readable = Readable;
  Stream.Writable = Writable;
  Stream.Duplex = Duplex;
  Stream.Transform = Transform;
  Stream.PassThrough = PassThrough;
  Stream.Stream = Stream;
  Stream.finished = finished;
  Stream.pipeline = pipeline;
  Stream.promises = {
    finished: (stream) => new Promise((res, rej) => finished(stream, (e) => (e ? rej(e) : res()))),
    pipeline: function () {
      const args = Array.prototype.slice.call(arguments);
      return new Promise((res, rej) => {
        args.push((e) => (e ? rej(e) : res()));
        pipeline.apply(null, args);
      });
    },
  };

  module.exports = Stream;
});
