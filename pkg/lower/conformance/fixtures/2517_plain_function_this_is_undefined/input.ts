// A plain function's this is whatever the call supplies, and a Go closure has no
// receiver slot for a call to supply anything through, so a strict body reads
// undefined. The unit covers the three spellings that reach that answer, a top-level
// declaration, a nested declaration, and a function expression, plus an arrow inside
// one, which has no this of its own and so reads the function's.
//
// The wrapper is the shape Node's test/common/index.js is built out of: a callback
// that reads this only to hand it straight back out to another call. It is here
// because reading this as a value and reading a property off it are different
// failures, and this half is the one 769 tests of Node's suite were waiting on.
"use strict";

function describe(): string {
  return typeof this;
}

function outer(): string {
  function inner(): string {
    return typeof this;
  }
  const expr = function (): string {
    const arrow = () => typeof this;
    return arrow();
  };
  return inner() + " " + expr();
}

function wrap(fn: (x: number) => number): (x: number) => number {
  return function (x: number): number {
    return typeof this === "undefined" ? fn(x) : -1;
  };
}

console.log(describe());
console.log(outer());
console.log(wrap((x: number) => x + 1)(41));
