// assert implements node:assert. The default export is the callable assert(),
// with the named checks hung off it, matching Node.

__bento_defineModule("assert", function (module, exports, require) {
  "use strict";

  class AssertionError extends Error {
    constructor(options) {
      options = options || {};
      super(options.message || "Assertion failed");
      this.name = "AssertionError";
      this.code = "ERR_ASSERTION";
      this.actual = options.actual;
      this.expected = options.expected;
      this.operator = options.operator;
    }
  }

  function fail(actual, expected, message, operator) {
    if (arguments.length === 1) { message = actual; actual = undefined; }
    throw new AssertionError({
      message: message || "Failed",
      actual: actual,
      expected: expected,
      operator: operator || "fail",
    });
  }

  function assert(value, message) {
    if (!value) {
      throw new AssertionError({
        message: message || "The expression evaluated to a falsy value:",
        actual: value,
        expected: true,
        operator: "==",
      });
    }
  }

  function deepEqual(a, b, strict) {
    if (strict ? a === b : a == b) return true;
    if (typeof a !== "object" || typeof b !== "object" || a === null || b === null) {
      return !strict && a == b;
    }
    if (a instanceof Date && b instanceof Date) return a.getTime() === b.getTime();
    if (a instanceof RegExp && b instanceof RegExp) return a.toString() === b.toString();
    const ka = Object.keys(a);
    const kb = Object.keys(b);
    if (ka.length !== kb.length) return false;
    for (const k of ka) {
      if (!Object.prototype.hasOwnProperty.call(b, k)) return false;
      if (!deepEqual(a[k], b[k], strict)) return false;
    }
    return true;
  }

  assert.AssertionError = AssertionError;
  assert.fail = fail;
  assert.ok = assert;

  assert.equal = function (a, b, m) {
    if (a != b) fail(a, b, m || (inspect(a) + " == " + inspect(b)), "==");
  };
  assert.notEqual = function (a, b, m) {
    if (a == b) fail(a, b, m || (inspect(a) + " != " + inspect(b)), "!=");
  };
  assert.strictEqual = function (a, b, m) {
    if (!Object.is(a, b)) fail(a, b, m || (inspect(a) + " === " + inspect(b)), "strictEqual");
  };
  assert.notStrictEqual = function (a, b, m) {
    if (Object.is(a, b)) fail(a, b, m || (inspect(a) + " !== " + inspect(b)), "notStrictEqual");
  };
  assert.deepEqual = function (a, b, m) {
    if (!deepEqual(a, b, false)) fail(a, b, m || "deepEqual", "deepEqual");
  };
  assert.notDeepEqual = function (a, b, m) {
    if (deepEqual(a, b, false)) fail(a, b, m || "notDeepEqual", "notDeepEqual");
  };
  assert.deepStrictEqual = function (a, b, m) {
    if (!deepEqual(a, b, true)) fail(a, b, m || "deepStrictEqual", "deepStrictEqual");
  };
  assert.notDeepStrictEqual = function (a, b, m) {
    if (deepEqual(a, b, true)) fail(a, b, m || "notDeepStrictEqual", "notDeepStrictEqual");
  };

  function matchError(err, expected) {
    if (expected === undefined) return true;
    if (typeof expected === "function") {
      if (expected === Error || Error.isPrototypeOf(expected)) return err instanceof expected;
      return expected(err) === true;
    }
    if (expected instanceof RegExp) return expected.test(String(err && err.message !== undefined ? err.message : err));
    if (typeof expected === "object") {
      for (const k of Object.keys(expected)) if (err[k] !== expected[k]) return false;
      return true;
    }
    return true;
  }

  assert.throws = function (fn, expected, message) {
    try {
      fn();
    } catch (err) {
      if (!matchError(err, expected)) throw err;
      return;
    }
    fail(undefined, expected, message || "Missing expected exception.", "throws");
  };
  assert.doesNotThrow = function (fn, expected, message) {
    try {
      fn();
    } catch (err) {
      if (matchError(err, expected)) fail(err, expected, message || "Got unwanted exception.", "doesNotThrow");
      throw err;
    }
  };
  assert.rejects = async function (fn, expected, message) {
    try {
      await (typeof fn === "function" ? fn() : fn);
    } catch (err) {
      if (!matchError(err, expected)) throw err;
      return;
    }
    fail(undefined, expected, message || "Missing expected rejection.", "rejects");
  };
  assert.doesNotReject = async function (fn, expected, message) {
    await (typeof fn === "function" ? fn() : fn);
  };
  assert.match = function (str, re, m) {
    if (!re.test(str)) fail(str, re, m || "match", "match");
  };
  assert.doesNotMatch = function (str, re, m) {
    if (re.test(str)) fail(str, re, m || "doesNotMatch", "doesNotMatch");
  };
  assert.ifError = function (err) {
    if (err) throw err;
  };

  function inspect(v) {
    try { return typeof v === "string" ? JSON.stringify(v) : __bento_inspect(v); }
    catch (e) { return String(v); }
  }

  assert.strict = assert;
  module.exports = assert;
});
