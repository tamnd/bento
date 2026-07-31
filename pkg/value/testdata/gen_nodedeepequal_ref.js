// Generates the reference answers for the deep equality port in nodedeepequal.go.
//
// Run it with the Node the port targets and redirect into nodedeepequal_node24.json:
//
//   node pkg/value/testdata/gen_nodedeepequal_ref.js > pkg/value/testdata/nodedeepequal_node24.json
//
// Each case is a pair of values under a name the Go test builds the same pair for, so
// the two sides are matched by name rather than by position and a case added here
// fails the Go test until it is built there too. Each answer is recorded for all
// three modes the port carries: the strict comparison util.isDeepStrictEqual makes,
// the loose one assert.deepEqual makes, and the strict one with the prototype check
// dropped, which is assert's skipPrototype option.
//
// Every value is one bento can box into a value.Value. Date, the typed arrays and
// the boxed primitives are left out because they have no boxed form yet, and a
// revoked proxy is left out because Array.isArray throws on one, so the pair has no
// answer to record rather than a false one. Map and Set are in: they box now, and
// their comparison is the one with rules of its own, since an unordered container
// has to pair each entry against a candidate rather than walk two lists in step.

'use strict';
const assert = require('assert');
const util = require('util');

// Shared references, for the cases that are about identity rather than shape.
const sym = Symbol('s');
const tagged = Symbol('t');
const fn = function named() {};

// Two classes of the same shape, so a pair of instances differs only in the
// constructor the strict comparison reads.
class Point {
  constructor(v) {
    this.v = v;
  }
}
class Other {
  constructor(v) {
    this.v = v;
  }
}

const nullProtoWithKey = () => {
  const o = Object.create(null);
  o.a = 1;
  return o;
};

const hiddenKey = () => {
  const o = { a: 1 };
  Object.defineProperty(o, 'hidden', { value: 2, enumerable: false });
  return o;
};

const arrayWithNamed = () => {
  const a = [1];
  a.x = 1;
  return a;
};

const regexpWithNamed = () => {
  const r = /a/;
  r.x = 1;
  return r;
};

const movedRegExp = () => {
  const r = /a/g;
  r.lastIndex = 1;
  return r;
};

const withCode = (code) => Object.assign(new Error('x'), { code });

const selfRef = () => {
  const o = { a: 1 };
  o.self = o;
  return o;
};

const selfRefArray = () => {
  const a = [1];
  a.push(a);
  return a;
};

const mutual = () => {
  const a = { a: 1 };
  const b = { a: 1 };
  a.self = b;
  b.self = a;
  return [a, b];
};

const deeperCycle = () => {
  const outer = { a: 1 };
  outer.self = { a: 1, self: outer };
  return outer;
};

const [mutualA, mutualB] = mutual();
const shared = { v: 1 };

const mapWithNamed = () => {
  const m = new Map([[1, 2]]);
  m.x = 1;
  return m;
};

const selfRefMap = () => {
  const m = new Map();
  m.set('self', m);
  return m;
};

const selfRefSet = () => {
  const s = new Set();
  s.add(s);
  return s;
};

const cases = {
  // Primitives, which every comparison settles before it looks at an object.
  'identical numbers': [1, 1],
  'different numbers': [1, 2],
  'zero and negative zero': [0, -0],
  'negative zero twice': [-0, -0],
  'nan and nan': [NaN, NaN],
  'nan and number': [NaN, 1],
  'number and numeric string': [1, '1'],
  'number and true': [1, true],
  'null and undefined': [null, undefined],
  'null and null': [null, null],
  'empty string and zero': ['', 0],
  'identical strings': ['a', 'a'],
  'different strings': ['a', 'b'],
  'identical bigints': [1n, 1n],
  'bigint and number': [1n, 1],
  'same symbol': [sym, sym],
  'different symbols': [Symbol('a'), Symbol('a')],
  'same function': [fn, fn],
  'two functions': [function () {}, function () {}],
  'object and number': [{}, 1],
  'null and object': [null, {}],
  'object and null': [{}, null],

  // Plain objects.
  'empty objects': [{}, {}],
  'same one key': [{ a: 1 }, { a: 1 }],
  'different value': [{ a: 1 }, { a: 2 }],
  'loose value': [{ a: 1 }, { a: '1' }],
  'extra key': [{ a: 1 }, { a: 1, b: 2 }],
  'missing key': [{ a: 1, b: 2 }, { a: 1 }],
  'key order': [{ a: 1, b: 2 }, { b: 2, a: 1 }],
  'nested objects': [{ a: { b: { c: 1 } } }, { a: { b: { c: 1 } } }],
  'nested difference': [{ a: { b: { c: 1 } } }, { a: { b: { c: 2 } } }],
  'undefined value and missing key': [{ a: undefined }, {}],
  'undefined values': [{ a: undefined }, { a: undefined }],
  'null prototypes': [nullProtoWithKey(), nullProtoWithKey()],
  'null prototype and plain': [nullProtoWithKey(), { a: 1 }],
  'non enumerable key ignored': [hiddenKey(), { a: 1 }],
  'accessor and data': [{ get a() { return 1; } }, { a: 1 }],
  'accessor different': [{ get a() { return 1; } }, { a: 2 }],
  'same symbol key': [{ [sym]: 1 }, { [sym]: 1 }],
  'symbol key one side': [{ [sym]: 1 }, {}],
  'symbol key different value': [{ [sym]: 1 }, { [sym]: 2 }],
  'different symbol keys': [{ [sym]: 1 }, { [tagged]: 1 }],
  'to string tag same': [{ [Symbol.toStringTag]: 'X' }, { [Symbol.toStringTag]: 'X' }],
  'to string tag different': [{ [Symbol.toStringTag]: 'X' }, { [Symbol.toStringTag]: 'Y' }],
  'to string tag one side': [{ [Symbol.toStringTag]: 'X' }, {}],
  'function property same': [{ f: fn }, { f: fn }],
  'function property different': [{ f: function () {} }, { f: function () {} }],

  // Prototypes, which only the strict comparison looks at.
  'same class instances': [new Point(1), new Point(1)],
  'different classes': [new Point(1), new Other(1)],
  'class instance and plain object': [new Point(1), { v: 1 }],

  // Arrays.
  'empty arrays': [[], []],
  'same elements': [[1, 2], [1, 2]],
  'different length': [[1], [1, 2]],
  'different element': [[1, 2], [1, 3]],
  'loose element': [[1], ['1']],
  'nested arrays': [[[1], [2]], [[1], [2]]],
  'array and object': [[], {}],
  'object and array': [{}, []],
  'array named property one side': [arrayWithNamed(), [1]],
  'array named property both': [arrayWithNamed(), arrayWithNamed()],
  'hole and undefined element': [[, 1], [undefined, 1]],
  'holes both sides': [[, 1], [, 1]],
  'hole and value': [[, 1], [1, 1]],
  'null and undefined element': [[null], [undefined]],
  'array of objects': [[{ a: 1 }], [{ a: 1 }]],

  // Regexps, which are their pattern, their flags and their position.
  'same regexps': [/ab+c/gi, /ab+c/gi],
  'different source': [/a/, /b/],
  'different flags': [/a/g, /a/i],
  'different last index': [movedRegExp(), /a/g],
  'regexp and object': [/a/, {}],
  'regexp named property both': [regexpWithNamed(), regexpWithNamed()],
  'regexp named property one side': [regexpWithNamed(), /a/],

  // Errors, whose stack is deliberately not part of the comparison.
  'same errors': [new Error('x'), new Error('x')],
  'different messages': [new Error('x'), new Error('y')],
  'different error names': [new Error('x'), new TypeError('x')],
  'error and plain object': [new Error('x'), { name: 'Error', message: 'x' }],
  'plain object and error': [{ name: 'Error', message: 'x' }, new Error('x')],
  'errors with same code': [withCode('E'), withCode('E')],
  'errors with different code': [withCode('E'), withCode('F')],
  'aggregate errors': [new AggregateError([new Error('a')], 'm'),
                       new AggregateError([new Error('a')], 'm')],
  'aggregate different reasons': [new AggregateError([new Error('a')], 'm'),
                                  new AggregateError([new Error('b')], 'm')],

  // Cycles, which is what the memo set is for. The shared case is the one that says
  // the memo has to forget a pair once it is done with it: the same object appears
  // under two keys, and if it stayed in the memo the second visit would look like a
  // cycle against a value it was never compared to.
  'shared subobject one side': [{ x: shared, y: shared }, { x: { v: 1 }, y: { v: 1 } }],
  'deep nesting': [{ a: { b: { c: { d: { e: 1 } } } } }, { a: { b: { c: { d: { e: 1 } } } } }],
  'deep nesting difference': [{ a: { b: { c: { d: { e: 1 } } } } }, { a: { b: { c: { d: { e: 2 } } } } }],
  'self referencing objects': [selfRef(), selfRef()],
  'mutually referencing objects': [mutualA, mutualB],
  'cycle and deeper cycle': [selfRef(), deeperCycle()],
  'self referencing arrays': [selfRefArray(), selfRefArray()],

  // Maps, which are unordered, so an entry is paired against a candidate rather
  // than lined up by position, and a key that is not a primitive has to be searched
  // for by shape.
  'empty maps': [new Map(), new Map()],
  'same map entries': [new Map([[1, 2]]), new Map([[1, 2]])],
  'map entries out of order': [new Map([[1, 2], [3, 4]]), new Map([[3, 4], [1, 2]])],
  'map different size': [new Map([[1, 2]]), new Map([[1, 2], [3, 4]])],
  'map different value': [new Map([[1, 2]]), new Map([[1, 3]])],
  'map different key': [new Map([[1, 2]]), new Map([[3, 2]])],
  'map loose key': [new Map([[1, 2]]), new Map([['1', 2]])],
  'map loose value': [new Map([[1, 2]]), new Map([[1, '2']])],
  'map nan key': [new Map([[NaN, 1]]), new Map([[NaN, 1]])],
  'map zero keys': [new Map([[0, 1]]), new Map([[-0, 1]])],
  'map object keys': [new Map([[{ a: 1 }, 1]]), new Map([[{ a: 1 }, 1]])],
  'map object keys different': [new Map([[{ a: 1 }, 1]]), new Map([[{ a: 2 }, 1]])],
  'map object values': [new Map([[1, { a: 1 }]]), new Map([[1, { a: 1 }]])],
  'map undefined value': [new Map([['a', undefined]]), new Map([['a', undefined]])],
  'map undefined value other key': [new Map([['a', undefined]]), new Map([['b', undefined]])],
  'nested maps': [new Map([[1, new Map([[2, 3]])]]), new Map([[1, new Map([[2, 3]])]])],
  'map and object': [new Map(), {}],
  'object and map': [{}, new Map()],
  'map and set': [new Map(), new Set()],
  'map named property both': [mapWithNamed(), mapWithNamed()],
  'map named property one side': [mapWithNamed(), new Map([[1, 2]])],
  'self referencing maps': [selfRefMap(), selfRefMap()],

  // Sets, which are the same problem with one column instead of two.
  'empty sets': [new Set(), new Set()],
  'same set members': [new Set([1, 2]), new Set([1, 2])],
  'set members out of order': [new Set([1, 2]), new Set([2, 1])],
  'set different size': [new Set([1]), new Set([1, 2])],
  'set different member': [new Set([1]), new Set([2])],
  'set loose member': [new Set([1]), new Set(['1'])],
  'set nan member': [new Set([NaN]), new Set([NaN])],
  'set object members': [new Set([{ a: 1 }]), new Set([{ a: 1 }])],
  'set object members different': [new Set([{ a: 1 }]), new Set([{ a: 2 }])],
  'set and array': [new Set(), []],
  'array and set': [[], new Set()],
  'self referencing sets': [selfRefSet(), selfRefSet()],

  // Proxies, which are compared as whatever their traps answer.
  'proxy over object and object': [new Proxy({ a: 1 }, {}), { a: 1 }],
  'proxy over object and different object': [new Proxy({ a: 1 }, {}), { a: 2 }],
  'proxy over array and array': [new Proxy([1], {}), [1]],
};

// answers records one pair's verdict in each mode. The loose and skipPrototype modes
// have no predicate of their own, so they are read off the assertion that uses them:
// a throw is a difference and a return is equality, and anything that is not an
// assertion failure is a bug in the case rather than an answer.
const answers = (a, b) => ({
  strict: util.isDeepStrictEqual(a, b),
  loose: passes(() => assert.deepEqual(a, b)),
  skipPrototype: passes(() => new assert.Assert({ skipPrototype: true }).deepStrictEqual(a, b)),
});

function passes(run) {
  try {
    run();
    return true;
  } catch (e) {
    if (e.code !== 'ERR_ASSERTION') {
      throw e;
    }
    return false;
  }
}

const out = {};
for (const [name, [a, b]] of Object.entries(cases)) {
  out[name] = answers(a, b);
}
process.stdout.write(JSON.stringify(out, null, 2) + '\n');
