// Generates the reference output for the util.inspect port in nodeinspect.go.
//
// Run it with the Node the port targets and redirect into nodeinspect_node24.json:
//
//   node pkg/value/testdata/gen_nodeinspect_ref.js > pkg/value/testdata/nodeinspect_node24.json
//
// Each case has a name the Go test builds the same value for, so the two sides are
// matched by name rather than by position and a case added here is a compile error
// on the Go side until it is built there too. Every value is one bento can box into
// a value.Value; Map, Set, Date and the typed arrays are left out because they have
// no boxed form yet, not because Node renders them uninterestingly.

'use strict';
const util = require('util');

const circularObject = { a: 1 };
circularObject.self = circularObject;
const circularArray = [1];
circularArray.push(circularArray);
const sharedNested = { n: 1 };
const twoRefs = { a: sharedNested, b: sharedNested };
const deepCircular = { a: { b: {} } };
deepCircular.a.b.back = deepCircular;

const arrayWithProp = [1, 2];
arrayWithProp.x = 5;

const sparse = [1, , 3];
const sparseTail = [1, , , ,];

const nullProto = Object.create(null);
const nullProtoWithKeys = Object.create(null);
nullProtoWithKeys.a = 1;

function Named() {}
const classInstance = new Named();
classInstance.a = 1;

const stacklessError = (e) => {
  e.stack = e.name + (e.message ? ': ' + e.message : '');
  return e;
};

const symbolKeyed = {};
symbolKeyed[Symbol('k')] = 1;
symbolKeyed.plain = 2;

const cases = {
  // Primitives on their own.
  'undefined': undefined,
  'null': null,
  'true': true,
  'false': false,
  'zero': 0,
  'negative zero': -0,
  'integer': 42,
  'fraction': 1.5,
  'nan': NaN,
  'infinity': Infinity,
  'negative infinity': -Infinity,
  'large number': 1e21,
  'bigint': 1n,
  'negative bigint': -12345678901234567890n,
  'string': 'hi',
  'empty string': '',
  'string with single quote': "he'llo",
  'string with both quotes': `he'llo "there"`,
  'string with every quote': 'a\'b"c`d',
  'string with template opener': 'a\'b"c${d}',
  'string with newline': 'a\nb',
  'string with tab and backslash': 'a\tb\\c',
  'string with control char': 'ab',
  'string with del': 'ab',
  'string with lone surrogate': 'a\ud800b',
  'string with surrogate pair': 'a\u{1f600}b',
  'long single line string': 'x'.repeat(100),
  'long multiline string': ('line one is quite long here\n'.repeat(4)),
  'symbol': Symbol('s'),
  'symbol no description': Symbol(),

  // Plain objects.
  'empty object': {},
  'one key': { a: 1 },
  'two keys': { a: 1, b: 'x' },
  'numeric and odd keys': { 'a-b': 1, 2: 3, valid_id: 4 },
  'dollar key': { $a: 1 },
  'undefined and null values': { a: undefined, b: null },
  'seven short keys': { a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7 },
  'three long keys': {
    longKeyName1: 'aaaaaaaaaaaaaaaaaaaa',
    longKeyName2: 'bbbbbbbbbbbbbbbbbbbb',
    longKeyName3: 'cccccccccccccccccccc',
  },
  'nested past depth': { a: { b: { c: { d: 1 } } } },
  'nested at depth': { a: { b: { c: 1 } } },
  'null prototype': nullProto,
  'null prototype with keys': nullProtoWithKeys,
  'constructor named': classInstance,
  'symbol keyed': symbolKeyed,
  'nested string quoting': { s: "he'llo" },
  'nested newline string': { s: 'a\nb' },
  'bigint value': { a: 1n },
  'negative zero value': { n: -0 },
  'proto key': { ['__proto__']: 1 },

  // Arrays.
  'empty array': [],
  'three numbers': [1, 2, 3],
  'nested arrays past depth': [[1, [2, [3, [4]]]]],
  'array with named property': arrayWithProp,
  'sparse array': sparse,
  'sparse tail': sparseTail,
  'eight numbers': [0, 1, 2, 3, 4, 5, 6, 7],
  'seven numbers': [0, 1, 2, 3, 4, 5, 6],
  'twenty numbers': Array.from({ length: 20 }, (_, i) => i),
  'hundred and twenty numbers': Array.from({ length: 120 }, (_, i) => i),
  'ten strings': Array.from({ length: 10 }, (_, i) => 'item' + i),
  'array of objects': [{ a: 1 }, { b: 2 }],
  'mixed array': [1, 'two', true, null, undefined],

  // Functions and regexps.
  // Wrapped so named evaluation does not hand the arrow the property key as its
  // name, which is what makes it anonymous in the first place.
  'anonymous function': (function () { return () => {}; })(),
  'named function': function foo() {},
  'function with property': Object.assign(function bar() {}, { x: 1 }),
  'regexp': /ab+c/gi,
  'empty regexp': new RegExp(''),
  'regexp in object': { r: /a/ },

  // Errors, with the stack reduced to what a bento error carries.
  'error': stacklessError(new Error('x')),
  'error no message': stacklessError(new Error()),
  'type error with code': Object.assign(stacklessError(new TypeError('bad')), { code: 'E1' }),
  'error in array': [stacklessError(new Error('x'))],
  'error with properties': Object.assign(stacklessError(new Error('x')), { foo: 1, bar: 'y' }),

  // Cycles.
  'circular object': circularObject,
  'circular array': circularArray,
  'deep circular': deepCircular,
  'two references to one object': twoRefs,
};

const out = {};
for (const [name, value] of Object.entries(cases)) {
  out[name] = util.inspect(value);
}
process.stdout.write(JSON.stringify(out, null, 2) + '\n');
