// The &&= and ||= compound assignments short-circuit on the target's JavaScript
// truthiness, not a boolean: ||= assigns when the target is falsy, &&= when it is
// truthy. This runs the same ToBoolean each type uses in a condition, so a number
// tests against zero and NaN, a string against empty, and an object is always
// truthy so its &&= always assigns and its ||= never does.

function orNum(x: number): number {
  x ||= 7;
  return x;
}

function andNum(x: number): number {
  x &&= 7;
  return x;
}

function orStr(s: string): string {
  s ||= "fallback";
  return s;
}

function andObj(o: { a: number }): number {
  o &&= { a: 9 };
  return o.a;
}

console.log(orNum(0));
console.log(orNum(3));
console.log(andNum(0));
console.log(andNum(5));
console.log(orStr(""));
console.log(andObj({ a: 1 }));
