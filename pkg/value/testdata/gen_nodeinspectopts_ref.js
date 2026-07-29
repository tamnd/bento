// Generates the reference output for util.inspect's options, the surface
// util.inspect(value, options) and util.formatWithOptions reach in nodeinspect.go.
//
// Run it with the Node the port targets and redirect into
// nodeinspectopts_node24.json:
//
//   node pkg/value/testdata/gen_nodeinspectopts_ref.js > pkg/value/testdata/nodeinspectopts_node24.json
//
// The plain-defaults cases live in gen_nodeinspect_ref.js; this file is only about
// what an option changes. Each case names the value and the options the Go test
// builds the same way, matched by name.
//
// Two option effects are deliberately absent, because bento's value model cannot
// express them yet and a reference case would only record a known failure:
// showHidden on a function (Node lists its length, name, prototype and the rest of
// the hidden slots, which a boxed function does not carry) and showHidden's walk
// into prototype properties (bento has no prototype objects for the built-ins).
//
// A getter that throws is left out for a different reason: Node prints the thrown
// error's stack inside the <Inspection threw (...)> and the stack names this file's
// absolute path, so the case would record where it was generated. A bento error
// carries no stack, so that case is a hand-written test instead.

'use strict';
const util = require('util');

const hiddenProp = { a: 1 };
Object.defineProperty(hiddenProp, 'hidden', { value: 2, enumerable: false });

const hiddenSymbol = { a: 1 };
Object.defineProperty(hiddenSymbol, Symbol('s'), { value: 2, enumerable: false });

const accessors = { plain: 1 };
Object.defineProperty(accessors, 'onlyGet', { get() { return 5; }, enumerable: true });
Object.defineProperty(accessors, 'onlySet', { set(v) {}, enumerable: true });
Object.defineProperty(accessors, 'both', { get() { return 6; }, set(v) {}, enumerable: true });
Object.defineProperty(accessors, 'objectGet', { get() { return { deep: 1 }; }, enumerable: true });

const deep = { a: { b: { c: { d: { e: 1 } } } } };
const wide = { aaa: 1, bbb: 2, ccc: 3 };
const numbers = Array.from({ length: 8 }, (_, i) => i);
const longString = 'x'.repeat(20);
const proxied = new Proxy({ a: 1 }, { get(t, k) { return t[k]; } });

const cases = {
  'show hidden array': { value: [1, 2, 3], options: { showHidden: true } },
  'show hidden array of one': { value: ['a'], options: { showHidden: true } },
  'show hidden non enumerable': { value: hiddenProp, options: { showHidden: true } },
  'non enumerable hidden by default': { value: hiddenProp, options: {} },
  'show hidden symbol': { value: hiddenSymbol, options: { showHidden: true } },
  'non enumerable symbol hidden by default': { value: hiddenSymbol, options: {} },
  'show hidden nested array': { value: { a: [1] }, options: { showHidden: true } },
  'show hidden as second positional': { value: [1, 2], options: true },

  'sorted': { value: { c: 1, a: 2, b: 3 }, options: { sorted: true } },
  'sorted array keeps order': { value: [3, 1, 2], options: { sorted: true } },
  'sorted nested': { value: { z: { y: 1, x: 2 } }, options: { sorted: true } },
  'sorted with hidden keys': { value: hiddenProp, options: { sorted: true, showHidden: true } },
  'sorted comparator': { value: { a: 1, b: 2, c: 3 }, options: { sorted: (a, b) => (a < b ? 1 : -1) } },

  'getters off': { value: accessors, options: {} },
  'getters all': { value: accessors, options: { getters: true } },
  'getters get': { value: accessors, options: { getters: 'get' } },
  'getters set': { value: accessors, options: { getters: 'set' } },

  'numeric separator integer': { value: 1234567, options: { numericSeparator: true } },
  'numeric separator negative': { value: -1234567, options: { numericSeparator: true } },
  'numeric separator fraction': { value: -1234567.891, options: { numericSeparator: true } },
  'numeric separator small': { value: 123, options: { numericSeparator: true } },
  'numeric separator exponent': { value: 1e21, options: { numericSeparator: true } },
  'numeric separator infinity': { value: Infinity, options: { numericSeparator: true } },
  'numeric separator nan': { value: NaN, options: { numericSeparator: true } },
  'numeric separator bigint': { value: 1234567n, options: { numericSeparator: true } },
  'numeric separator in array': { value: [1234567], options: { numericSeparator: true } },

  'compact false': { value: wide, options: { compact: false } },
  'compact false nested': { value: { a: { b: 1 } }, options: { compact: false } },
  'compact false array': { value: numbers, options: { compact: false } },
  'compact true': { value: wide, options: { compact: true } },
  'compact true nested': { value: deep, options: { compact: true } },
  'compact true long': { value: { a: longString, b: longString }, options: { compact: true } },
  'compact one': { value: wide, options: { compact: 1 } },
  'compact one deep': { value: { a: { b: { c: 1 } } }, options: { compact: 1 } },

  'depth null': { value: deep, options: { depth: null } },
  'depth three': { value: deep, options: { depth: 3 } },
  'depth negative': { value: { a: 1 }, options: { depth: -1 } },
  'depth infinity': { value: deep, options: { depth: Infinity } },

  'max array length zero': { value: [1, 2, 3], options: { maxArrayLength: 0 } },
  'max array length one': { value: [1, 2, 3], options: { maxArrayLength: 1 } },
  'max array length null': { value: numbers, options: { maxArrayLength: null } },
  'max string length': { value: longString, options: { maxStringLength: 5 } },
  'max string length zero': { value: longString, options: { maxStringLength: 0 } },
  'max string length in object': { value: { s: longString }, options: { maxStringLength: 5 } },

  'break length small': { value: wide, options: { breakLength: 10 } },
  'break length large': { value: numbers, options: { breakLength: 1000 } },
  'break length one': { value: { a: 1 }, options: { breakLength: 1 } },

  'proxy hidden by default': { value: proxied, options: {} },
  'show proxy': { value: proxied, options: { showProxy: true } },
  'show proxy nested': { value: { p: proxied }, options: { showProxy: true } },

  'depth as positional': { value: deep, options: undefined, depth: 0 },
};

const out = {};
for (const [name, spec] of Object.entries(cases)) {
  out[name] = 'depth' in spec ?
    util.inspect(spec.value, spec.options, spec.depth) :
    util.inspect(spec.value, spec.options);
}
process.stdout.write(JSON.stringify(out, null, 2) + '\n');
