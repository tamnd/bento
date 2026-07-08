// Equality on two operands of the same non-primitive reference type is object
// identity, not a structural compare. Two distinct literals are never the same
// reference, an alias of one shares its reference, and the loose == and != agree
// with === and !== over objects since neither side is a primitive to coerce. Both
// operands lower to a Go pointer, so the comparison is a pointer identity check.

const a = { x: 1 };
const b = { x: 1 };
const c = a;

// Distinct literals are different references, an alias is the same reference.
console.log(a === b);
console.log(a === c);
console.log(a !== b);

// Loose equality over two objects is the same identity, no coercion runs.
console.log(a == b);
console.log(a != b);

// Arrays are references too, so the same identity rules hold.
const p = [1, 2];
const q = [1, 2];
const r = p;
console.log(p === q);
console.log(p === r);
