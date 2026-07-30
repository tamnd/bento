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
