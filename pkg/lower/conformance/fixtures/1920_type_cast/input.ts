// A type cast carries no runtime effect, so it erases to its inner value: the
// test262 assert prelude binds `const assert = function () {} as Assert`, and a
// cast is what lets that function value take the prelude's callable-object type.
// The forms exercised here are the `as` cast over a plain value, the same cast
// bridging an unknown value down to a number, and the angle-bracket assertion.
const n = 5 as number;
console.log(n + 1);

const raw: unknown = 7;
console.log((raw as number) + 1);

const m = <number>10;
console.log(m + 2);
