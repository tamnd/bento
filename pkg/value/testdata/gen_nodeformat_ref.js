// Generates the reference output for the util.format port in nodeformat.go.
//
// Run it with the Node the port targets and redirect into nodeformat_node24.json:
//
//   node pkg/value/testdata/gen_nodeformat_ref.js > pkg/value/testdata/nodeformat_node24.json
//
// Each case has a name the Go test builds the same arguments for, so the two sides
// are matched by name rather than by position and a case added here fails the Go
// test until it is built there too. A case with an "options" key goes through
// util.formatWithOptions instead of util.format.
//
// Every argument is one bento can box into a value.Value. Map, Set, Date and the
// typed arrays are left out because they have no boxed form yet.

'use strict';
const util = require('util');

const circular = { a: 1 };
circular.self = circular;

const nullProto = Object.create(null);

const stackless = (e) => {
  e.stack = e.name + (e.message ? ': ' + e.message : '');
  return e;
};

const cases = {
  // Nothing to format.
  'no arguments': { args: [] },
  'one string': { args: ['hello'] },
  'one string with escape': { args: ['a%%b'] },
  'one string with specifier': { args: ['%s'] },
  'first argument not a string': { args: [{ a: 1 }, 'b'] },
  'first argument a number': { args: [1, 2] },

  // %s.
  's string': { args: ['%s', 'a'] },
  's number': { args: ['%s', 42] },
  's negative zero': { args: ['%s', -0] },
  's fraction': { args: ['%s', -1234567.891] },
  's nan': { args: ['%s', NaN] },
  's bigint': { args: ['%s', 10n] },
  's boolean': { args: ['%s', true] },
  's null': { args: ['%s', null] },
  's undefined': { args: ['%s', undefined] },
  's symbol': { args: ['%s', Symbol('s')] },
  's object': { args: ['%s', { a: 1 }] },
  's nested object': { args: ['%s', { a: { b: { c: 1 } } }] },
  's array': { args: ['%s', [1, 2]] },
  's nested array': { args: ['%s', [[1], [2]]] },
  's own toString': { args: ['%s', { toString() { return 'own'; } }] },
  's own toPrimitive': { args: ['%s', { [Symbol.toPrimitive]() { return 'prim'; } }] },
  's null prototype': { args: ['%s', nullProto] },
  's regexp': { args: ['%s', /ab+c/gi] },
  's error': { args: ['%s', stackless(new TypeError('bad'))] },
  's two': { args: ['%s %s', 'a', 'b'] },
  's missing argument': { args: ['%s %s', 'a'] },
  's then text': { args: ['%s: done', 'a'] },
  's leading text': { args: ['value is %s', 'a'] },

  // %d, %i, %f.
  'd integer': { args: ['%d', 42] },
  'd negative zero': { args: ['%d', -0] },
  'd string': { args: ['%d', '10'] },
  'd hex string': { args: ['%d', '0x10'] },
  'd not a number': { args: ['%d', 'abc'] },
  'd bigint': { args: ['%d', 10n] },
  'd symbol': { args: ['%d', Symbol('s')] },
  'd null': { args: ['%d', null] },
  'd undefined': { args: ['%d', undefined] },
  'd object': { args: ['%d', { a: 1 }] },
  'd array of one': { args: ['%d', [5]] },
  'i trailing text': { args: ['%i', '42.9px'] },
  'i hex string': { args: ['%i', '0x10'] },
  'i fraction': { args: ['%i', 42.9] },
  'i bigint': { args: ['%i', 10n] },
  'i symbol': { args: ['%i', Symbol('s')] },
  'i not a number': { args: ['%i', 'abc'] },
  'f trailing text': { args: ['%f', '3.5abc'] },
  'f leading dot': { args: ['%f', '.5'] },
  'f bigint': { args: ['%f', 10n] },
  'f symbol': { args: ['%f', Symbol('s')] },
  'f not a number': { args: ['%f', 'abc'] },

  // %j.
  'j object': { args: ['%j', { a: 1 }] },
  'j nested': { args: ['%j', [1, { a: 2 }]] },
  'j string': { args: ['%j', 'str'] },
  'j number': { args: ['%j', 1.5] },
  'j undefined': { args: ['%j', undefined] },
  'j function': { args: ['%j', function foo() {}] },
  'j symbol': { args: ['%j', Symbol('s')] },
  'j circular': { args: ['%j', circular] },

  // %o and %O.
  'o array': { args: ['%o', [1, 2, 3]] },
  'o object': { args: ['%o', { a: 1 }] },
  'o deep': { args: ['%o', { a: { b: { c: { d: { e: 1 } } } } }] },
  'O object': { args: ['%O', { a: 1 }] },
  'O deep': { args: ['%O', { a: { b: { c: { d: 1 } } } }] },
  'O string': { args: ['%O', 'a'] },

  // %c and %%.
  'c consumes an argument': { args: ['%c', 'css', 'rest'] },
  'c inline': { args: ['a%cb', 'css'] },
  'percent with argument': { args: ['%%', 'x'] },
  'percent before specifier': { args: ['%% %s', 'x'] },
  'percent glued to specifier': { args: ['%%%s', 'x'] },
  'unknown specifier': { args: ['%z', 1] },
  'trailing percent': { args: ['100%', 'x'] },
  'lone percent': { args: ['%', 'x'] },
  'specifier then percent': { args: ['%s%', 'x'] },

  // Leftover arguments.
  'leftovers of every kind': { args: ['%s', 'a', 'b', 1, { x: 1 }] },
  'leftover null and undefined': { args: ['%s', null, undefined, true] },
  'leftover with no specifier': { args: ['plain', 'a', 1] },

  // formatWithOptions.
  'options numeric separator d': { options: { numericSeparator: true }, args: ['%d', 1234567] },
  'options numeric separator s': { options: { numericSeparator: true }, args: ['%s', -1234567.891] },
  'options numeric separator i': { options: { numericSeparator: true }, args: ['%i', '1234567'] },
  'options numeric separator bigint': { options: { numericSeparator: true }, args: ['%d', 1234567n] },
  'options depth zero': { options: { depth: 0 }, args: ['%O', { a: { b: 1 } }] },
  'options depth one': { options: { depth: 1 }, args: ['%O', { a: { b: { c: 1 } } }] },
  'options empty': { options: {}, args: ['%s', { a: 1 }] },
  'options array': { options: [], args: ['%s', { a: 1 }] },
  'options break length': { options: { breakLength: 10 }, args: ['%O', { aaa: 1, bbb: 2, ccc: 3 }] },
  'options sorted': { options: { sorted: true }, args: ['%O', { c: 1, a: 2, b: 3 }] },
  'options max array length': { options: { maxArrayLength: 2 }, args: ['%O', [1, 2, 3, 4]] },
  'options leftover': { options: { depth: 0 }, args: ['leftover', { a: { b: 1 } }] },
};

const out = {};
for (const [name, spec] of Object.entries(cases)) {
  out[name] = 'options' in spec ?
    util.formatWithOptions(spec.options, ...spec.args) :
    util.format(...spec.args);
}
process.stdout.write(JSON.stringify(out, null, 2) + '\n');
