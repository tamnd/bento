// events implements node:events with the EventEmitter surface most code needs.

__bento_defineModule("events", function (module, exports, require) {
  "use strict";

  function EventEmitter() {
    if (!(this instanceof EventEmitter)) return new EventEmitter();
    this._events = Object.create(null);
    this._maxListeners = undefined;
  }

  EventEmitter.defaultMaxListeners = 10;

  EventEmitter.prototype.setMaxListeners = function (n) {
    this._maxListeners = n;
    return this;
  };
  EventEmitter.prototype.getMaxListeners = function () {
    return this._maxListeners === undefined ? EventEmitter.defaultMaxListeners : this._maxListeners;
  };

  function listenersFor(self, type) {
    if (!self._events[type]) self._events[type] = [];
    return self._events[type];
  }

  EventEmitter.prototype.addListener = function (type, listener, prepend) {
    if (typeof listener !== "function") throw new TypeError("listener must be a function");
    if (this._events["newListener"]) this.emit("newListener", type, listener);
    const list = listenersFor(this, type);
    if (prepend) list.unshift(listener);
    else list.push(listener);
    return this;
  };
  EventEmitter.prototype.on = EventEmitter.prototype.addListener;
  EventEmitter.prototype.prependListener = function (type, listener) {
    return this.addListener(type, listener, true);
  };

  EventEmitter.prototype.once = function (type, listener) {
    if (typeof listener !== "function") throw new TypeError("listener must be a function");
    const self = this;
    function wrapper() {
      self.removeListener(type, wrapper);
      return listener.apply(this, arguments);
    }
    wrapper.listener = listener;
    return this.on(type, wrapper);
  };
  EventEmitter.prototype.prependOnceListener = function (type, listener) {
    const self = this;
    function wrapper() {
      self.removeListener(type, wrapper);
      return listener.apply(this, arguments);
    }
    wrapper.listener = listener;
    return this.prependListener(type, wrapper);
  };

  EventEmitter.prototype.removeListener = function (type, listener) {
    const list = this._events[type];
    if (!list) return this;
    for (let i = list.length - 1; i >= 0; i--) {
      if (list[i] === listener || list[i].listener === listener) {
        list.splice(i, 1);
        this.emit("removeListener", type, listener);
        break;
      }
    }
    if (list.length === 0) delete this._events[type];
    return this;
  };
  EventEmitter.prototype.off = EventEmitter.prototype.removeListener;

  EventEmitter.prototype.removeAllListeners = function (type) {
    if (arguments.length === 0) {
      this._events = Object.create(null);
    } else {
      delete this._events[type];
    }
    return this;
  };

  EventEmitter.prototype.emit = function (type) {
    const list = this._events[type];
    const args = Array.prototype.slice.call(arguments, 1);
    if (!list || list.length === 0) {
      if (type === "error") {
        const err = args[0] instanceof Error ? args[0] : new Error("Unhandled error. (" + args[0] + ")");
        throw err;
      }
      return false;
    }
    const copy = list.slice();
    for (let i = 0; i < copy.length; i++) copy[i].apply(this, args);
    return true;
  };

  EventEmitter.prototype.listeners = function (type) {
    const list = this._events[type];
    return list ? list.map((l) => l.listener || l) : [];
  };
  EventEmitter.prototype.rawListeners = function (type) {
    const list = this._events[type];
    return list ? list.slice() : [];
  };
  EventEmitter.prototype.listenerCount = function (type) {
    const list = this._events[type];
    return list ? list.length : 0;
  };
  EventEmitter.prototype.eventNames = function () {
    return Object.keys(this._events);
  };

  EventEmitter.EventEmitter = EventEmitter;

  EventEmitter.once = function (emitter, name) {
    return new Promise((resolve, reject) => {
      function onEvent() {
        if (typeof emitter.removeListener === "function") emitter.removeListener("error", onError);
        resolve(Array.prototype.slice.call(arguments));
      }
      function onError(err) {
        emitter.removeListener(name, onEvent);
        reject(err);
      }
      emitter.once(name, onEvent);
      if (name !== "error" && typeof emitter.once === "function") emitter.once("error", onError);
    });
  };

  module.exports = EventEmitter;
});
