// gen_nodeassert_ref.js writes the message Node's assert raises for each named case,
// the ground truth pkg/value/nodeassert_test.go compares bento's port against.
//
// Run it with the Node the reference is named after and redirect it over the JSON:
//
//   node gen_nodeassert_ref.js > nodeassert_node24.json
//
// A case is a function that must throw. What is recorded is the message, the name and
// the code of what it threw, plus whether the message was generated, because those
// four are what a test that catches an assertion reads. A case that does not throw is
// recorded as such rather than skipped, so a case that stops failing shows up as a
// mismatch instead of disappearing.
//
// Colors are forced off. Node colors an assertion message when stderr is a color TTY,
// and bento never does, so the reference is taken in the uncolored shape a redirected
// run produces anyway.
//
// One case cannot be recorded: assert.throws(fn, Error) where fn threw a non-error. Node
// tests the expectation with Error.isPrototypeOf(expected), which is false for Error
// itself, so it calls Error as a validation function and reports the error that call
// returned, stack and all. The message carries absolute paths from the machine that ran
// the generator, so it is not ground truth anyone can compare against. It is a bento
// divergence with a test of its own instead.

'use strict';

const assert = require('assert');

// The shared values. A case that needs both sides to be the same reference names one
// of these on both sides, since every other builder below hands back a fresh value
// each time it is called and two structurally equal objects are not one object. They
// are what the identity-sensitive methods are tested with: notStrictEqual over two
// references to one object is the failure that reads "not to be reference-equal".
const sharedObject = { a: 1 };
const sharedNestedObject = { a: { b: { c: 1 } } };
const sharedArray = [1, 2, 3];
const sharedFunction = function foo() {};
const sharedError = new Error('a');

// The value builders. Each case names two of these rather than writing a literal, so
// the Go test can build the same values by the same names.
const values = {
  'shared object': () => sharedObject,
  'shared nested object': () => sharedNestedObject,
  'shared array': () => sharedArray,
  'shared function': () => sharedFunction,
  'shared error': () => sharedError,
  'zero': () => 0,
  'negative zero': () => -0,
  'one': () => 1,
  'two': () => 2,
  'nan': () => NaN,
  'true': () => true,
  'false': () => false,
  'null': () => null,
  'undefined': () => undefined,
  'string a': () => 'a',
  'string b': () => 'b',
  'string abcdefghij': () => 'abcdefghij',
  'string abcdefghik': () => 'abcdefghik',
  'long number': () => 1234567890123,
  'other long number': () => 1234567890124,
  'empty object': () => ({}),
  'object a1': () => ({ a: 1 }),
  'object a2': () => ({ a: 2 }),
  'object ab': () => ({ a: 1, b: 2 }),
  'object abc': () => ({ a: 1, b: 2, c: 3 }),
  'object out of order': () => ({ b: 2, a: 1 }),
  'nested object': () => ({ a: { b: { c: 1 } } }),
  'other nested object': () => ({ a: { b: { c: 2 } } }),
  'empty array': () => [],
  'array 123': () => [1, 2, 3],
  'array 124': () => [1, 2, 4],
  'array 1234': () => [1, 2, 3, 4],
  'ten line object': () => ({ a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7, h: 8 }),
  'ten line object changed': () => ({ a: 1, b: 2, c: 3, d: 4, e: 9, f: 6, g: 7, h: 8 }),
  'twenty line object': () => Object.fromEntries(Array.from({ length: 20 }, (_, i) => ['k' + i, i])),
  'twenty line object changed': () => Object.fromEntries(Array.from({ length: 20 }, (_, i) => ['k' + i, i === 19 ? 99 : i])),
  'error a': () => new Error('a'),
  'error b': () => new Error('b'),
  'regexp': () => /a+/,
  'function': () => function foo() {},
  'other function': () => function bar() {},

  // The throwers. throws and doesNotThrow take a function rather than a value, so their
  // first argument is one of these: each raises one particular thing, or nothing.
  'thrower nothing': () => () => {},
  'thrower error a': () => () => { throw new Error('a'); },
  'thrower error empty': () => () => { throw new Error(); },
  'thrower type error bad': () => () => { throw new TypeError('bad'); },
  'thrower error with code': () => () => { const e = new Error('a'); e.code = 'Y'; throw e; },
  'thrower number': () => () => { throw 42; },
  'thrower string': () => () => { throw 'boom'; },
  'thrower object': () => () => { throw {}; },

  // The expectations. An error class, a regexp over the string form of what was thrown,
  // a validation object compared key by key, and a validation function returning true
  // are the four kinds throws accepts.
  'ctor Error': () => Error,
  'ctor TypeError': () => TypeError,
  'ctor RangeError': () => RangeError,
  'regexp bad': () => /bad/,
  'regexp error a': () => /^Error: a$/,
  'validation true': () => () => true,
  'validation false': () => () => false,
  'validation one': () => () => 1,
  'validation named nope': () => function named() { return 'nope'; },
  'validation message a': () => (e) => e.message === 'a',
  'expect name': () => ({ name: 'TypeError' }),
  'expect name and message': () => ({ name: 'TypeError', message: 'other' }),
  'expect message regexp': () => ({ message: /bad/ }),
  'expect message regexp unmatched': () => ({ message: /nope/ }),
  'expect code': () => ({ code: 'X' }),
};

// The methods under test, by the name the Go test uses for them.
const methods = {
  'ok': (a) => assert.ok(a),
  'ok message': (a) => assert.ok(a, 'custom'),
  'equal': (a, b) => assert.equal(a, b),
  'notEqual': (a, b) => assert.notEqual(a, b),
  'strictEqual': (a, b) => assert.strictEqual(a, b),
  'strictEqual message': (a, b) => assert.strictEqual(a, b, 'custom'),
  'notStrictEqual': (a, b) => assert.notStrictEqual(a, b),
  'deepEqual': (a, b) => assert.deepEqual(a, b),
  'notDeepEqual': (a, b) => assert.notDeepEqual(a, b),
  'deepStrictEqual': (a, b) => assert.deepStrictEqual(a, b),
  'deepStrictEqual message': (a, b) => assert.deepStrictEqual(a, b, 'custom'),
  'notDeepStrictEqual': (a, b) => assert.notDeepStrictEqual(a, b),
  'ifError': (a) => assert.ifError(a),
  'fail': (a) => assert.fail(a),
  'fail nothing': () => assert.fail(),
  'match': (a, b) => assert.match(a, b),
  'doesNotMatch': (a, b) => assert.doesNotMatch(a, b),
  'throws': (a, b) => assert.throws(a, b),
  'throws message': (a, b) => assert.throws(a, b, 'custom'),
  'throws error message': (a, b) => assert.throws(a, b, new RangeError('as message')),
  'doesNotThrow': (a, b) => assert.doesNotThrow(a, b),
  'doesNotThrow message': (a, b) => assert.doesNotThrow(a, b, 'custom'),
};

// Each case is [method, actual, expected]. A case naming a value the method ignores
// still names it, so the Go side builds the same arguments.
const cases = [
  ['ok', 'false', 'undefined'],
  ['ok', 'zero', 'undefined'],
  ['ok', 'null', 'undefined'],
  ['ok', 'empty object', 'undefined'],
  ['ok message', 'false', 'undefined'],
  ['fail nothing', 'undefined', 'undefined'],
  ['fail', 'string a', 'undefined'],
  ['fail', 'null', 'undefined'],

  ['equal', 'one', 'two'],
  ['equal', 'string a', 'string b'],
  ['equal', 'nan', 'one'],
  ['equal', 'object a1', 'object a2'],
  ['equal', 'one', 'one'],
  ['equal', 'nan', 'nan'],
  ['notEqual', 'one', 'one'],
  ['notEqual', 'nan', 'nan'],
  ['notEqual', 'zero', 'false'],
  ['notEqual', 'shared object', 'shared object'],
  ['notEqual', 'one', 'two'],

  ['strictEqual', 'one', 'two'],
  ['strictEqual', 'zero', 'negative zero'],
  ['strictEqual', 'string a', 'string b'],
  ['strictEqual', 'string abcdefghij', 'string abcdefghik'],
  ['strictEqual', 'long number', 'other long number'],
  ['strictEqual', 'one', 'string a'],
  ['strictEqual', 'null', 'undefined'],
  ['strictEqual', 'object a1', 'object a1'],
  ['strictEqual', 'object a1', 'object a2'],
  ['strictEqual', 'empty object', 'empty object'],
  ['strictEqual', 'function', 'other function'],
  ['strictEqual', 'error a', 'error b'],
  ['strictEqual message', 'one', 'two'],
  ['strictEqual message', 'object a1', 'object a2'],
  ['strictEqual', 'one', 'one'],
  ['notStrictEqual', 'one', 'one'],
  ['notStrictEqual', 'nan', 'nan'],
  ['notStrictEqual', 'string a', 'string a'],
  ['notStrictEqual', 'shared nested object', 'shared nested object'],
  ['notStrictEqual', 'shared function', 'shared function'],
  ['notStrictEqual', 'shared error', 'shared error'],
  ['notStrictEqual', 'one', 'two'],

  ['deepEqual', 'object a1', 'object a2'],
  ['deepEqual', 'array 123', 'array 124'],
  ['deepEqual', 'object a1', 'array 123'],
  ['deepEqual', 'error a', 'error b'],
  ['notDeepEqual', 'object a1', 'object a1'],
  ['notDeepEqual', 'object ab', 'object out of order'],
  ['notDeepEqual', 'zero', 'false'],
  ['notDeepEqual', 'array 123', 'array 123'],
  ['notDeepEqual', 'object a1', 'object a2'],

  ['deepStrictEqual', 'object a1', 'object a2'],
  ['deepStrictEqual', 'object a1', 'object ab'],
  ['deepStrictEqual', 'object ab', 'object abc'],
  ['deepStrictEqual', 'object a1', 'object out of order'],
  ['deepStrictEqual', 'nested object', 'other nested object'],
  ['deepStrictEqual', 'array 123', 'array 124'],
  ['deepStrictEqual', 'array 123', 'array 1234'],
  ['deepStrictEqual', 'empty array', 'array 123'],
  ['deepStrictEqual', 'ten line object', 'ten line object changed'],
  ['deepStrictEqual', 'twenty line object', 'twenty line object changed'],
  ['deepStrictEqual', 'object a1', 'empty object'],
  ['deepStrictEqual', 'one', 'string a'],
  ['deepStrictEqual', 'error a', 'error b'],
  ['deepStrictEqual', 'array 123', 'object a1'],
  ['deepStrictEqual message', 'object a1', 'object a2'],
  ['deepStrictEqual message', 'one', 'two'],
  ['deepStrictEqual', 'object a1', 'object a1'],
  ['notDeepStrictEqual', 'object a1', 'object a1'],
  ['notDeepStrictEqual', 'object ab', 'object out of order'],
  ['notDeepStrictEqual', 'nested object', 'nested object'],
  ['notDeepStrictEqual', 'array 123', 'array 123'],
  ['notDeepStrictEqual', 'one', 'one'],
  ['notDeepStrictEqual', 'string a', 'string a'],
  ['notDeepStrictEqual', 'twenty line object', 'twenty line object'],
  ['notDeepStrictEqual', 'object a1', 'object a2'],

  ['ifError', 'error a', 'undefined'],
  ['ifError', 'one', 'undefined'],
  ['ifError', 'string a', 'undefined'],
  ['ifError', 'false', 'undefined'],
  ['ifError', 'object a1', 'undefined'],
  ['ifError', 'null', 'undefined'],
  ['ifError', 'undefined', 'undefined'],

  ['match', 'string b', 'regexp'],
  ['match', 'one', 'regexp'],
  ['match', 'string a', 'regexp'],
  ['doesNotMatch', 'string a', 'regexp'],
  ['doesNotMatch', 'string b', 'regexp'],

  // throws with no expectation: something has to be thrown and that is all.
  ['throws', 'thrower error a', 'undefined'],
  ['throws', 'thrower number', 'undefined'],
  ['throws', 'thrower nothing', 'undefined'],
  ['throws message', 'thrower nothing', 'undefined'],
  ['throws', 'one', 'undefined'],
  ['throws', 'null', 'undefined'],

  // An error class, matched by prototype in Node and by name here.
  ['throws', 'thrower type error bad', 'ctor TypeError'],
  ['throws', 'thrower type error bad', 'ctor Error'],
  ['throws', 'thrower error a', 'ctor TypeError'],
  ['throws', 'thrower error empty', 'ctor TypeError'],
  ['throws', 'thrower number', 'ctor TypeError'],
  ['throws', 'thrower object', 'ctor TypeError'],
  ['throws', 'thrower nothing', 'ctor RangeError'],
  ['throws message', 'thrower nothing', 'ctor Error'],
  ['throws message', 'thrower error a', 'ctor TypeError'],
  ['throws error message', 'thrower error a', 'ctor TypeError'],

  // A regexp, matched against the string form of what was thrown rather than its message.
  ['throws', 'thrower type error bad', 'regexp bad'],
  ['throws', 'thrower error a', 'regexp error a'],
  ['throws', 'thrower error a', 'regexp bad'],
  ['throws', 'thrower number', 'regexp bad'],

  // A validation object, key by key, and the placeholder diff a mismatch prints.
  ['throws', 'thrower type error bad', 'expect name'],
  ['throws', 'thrower type error bad', 'expect name and message'],
  ['throws', 'thrower error a', 'expect name'],
  ['throws', 'thrower type error bad', 'expect message regexp'],
  ['throws', 'thrower type error bad', 'expect message regexp unmatched'],
  ['throws', 'thrower error with code', 'expect code'],
  ['throws', 'thrower error a', 'expect code'],
  ['throws', 'thrower object', 'expect code'],
  ['throws', 'thrower number', 'expect code'],
  ['throws', 'thrower error a', 'empty object'],
  ['throws', 'thrower error a', 'error b'],
  ['throws message', 'thrower error a', 'expect code'],

  // A validation function, which has to return true rather than something truthy.
  ['throws', 'thrower error a', 'validation true'],
  ['throws', 'thrower error a', 'validation false'],
  ['throws', 'thrower error a', 'validation one'],
  ['throws', 'thrower error a', 'validation named nope'],
  ['throws', 'thrower error a', 'validation message a'],
  ['throws', 'thrower number', 'validation false'],

  // A string second argument is the message, and a message beside it is refused.
  ['throws', 'thrower error a', 'string a'],
  ['throws', 'thrower error a', 'string b'],
  ['throws', 'thrower string', 'string a'],
  ['throws message', 'thrower error a', 'string a'],

  // doesNotThrow, whose expectation names the error that would be a bug. Anything else
  // is rethrown as itself, which is the recorded Error rather than an assertion.
  ['doesNotThrow', 'thrower nothing', 'undefined'],
  ['doesNotThrow', 'thrower error a', 'undefined'],
  ['doesNotThrow', 'thrower number', 'undefined'],
  ['doesNotThrow message', 'thrower error a', 'undefined'],
  ['doesNotThrow', 'thrower type error bad', 'ctor TypeError'],
  ['doesNotThrow', 'thrower error a', 'ctor TypeError'],
  ['doesNotThrow message', 'thrower type error bad', 'ctor TypeError'],
  ['doesNotThrow', 'thrower error a', 'regexp error a'],
  ['doesNotThrow', 'thrower error a', 'regexp bad'],
  ['doesNotThrow', 'thrower error a', 'validation true'],
  ['doesNotThrow', 'thrower error a', 'validation false'],
  ['doesNotThrow', 'thrower error a', 'string a'],
  ['doesNotThrow', 'thrower error a', 'one'],
  ['doesNotThrow', 'thrower error a', 'expect code'],
  ['doesNotThrow', 'one', 'undefined'],
];

function record(method, actualName, expectedName) {
  const fn = methods[method];
  const a = values[actualName]();
  const b = values[expectedName]();
  try {
    fn(a, b);
    return { threw: false };
  } catch (e) {
    return {
      threw: true,
      name: e.name,
      code: e.code,
      message: e.message,
      generatedMessage: e.generatedMessage,
      operator: typeof e.operator === 'string' ? e.operator : String(e.operator),
    };
  }
}

const out = {};
for (const [method, actualName, expectedName] of cases) {
  const key = `${method}(${actualName}, ${expectedName})`;
  if (key in out) throw new Error(`duplicate case ${key}`);
  out[key] = record(method, actualName, expectedName);
}
process.stdout.write(JSON.stringify(out, null, 1) + '\n');
