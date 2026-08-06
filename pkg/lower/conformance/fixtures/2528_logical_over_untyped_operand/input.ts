// A value-returning && or || over an operand the checker has no type for is what a
// CommonJS program's guards are made of. `const os = require("os")` binds a box with no
// declaration behind it, so the checker gives the whole logical expression no type at
// all, and truthiness over a typeless node used to hand back. The lowering never needed
// the type: both operands box, value.And and value.Or pick the surviving one, and
// value.ToBoolean reads its truth, the same test either operand would get alone.
//
// The shape is recognized by what it lowers to rather than by what the checker calls
// it, which is also what lets it nest: the left operand of the outer && is itself a
// logical, so the outer one reads as a box exactly when the inner one does.

const os = require("os");
const path = require("path");

// argv always carries at least the runtime and the script, so this is true however the
// program was invoked, which keeps the fixture's output the same under any harness.
const twoArgs = process.argv.length >= 2;

// A logical in condition position, both operand orders, and a chain of three.
if (twoArgs && os) console.log("and-right");
if (os && twoArgs) console.log("and-left");
if (twoArgs && os && path) console.log("chain");

// Under a negation and inside parentheses, which is how a guard is usually written.
if (!(os || path)) console.log("unreachable");
else console.log("or-negated");

// The operator returns an operand, not a boolean, so the result lands in a binding
// whose Go slot is the box the operator answered.
const picked = twoArgs && os;
console.log(typeof picked);
const orPicked = os || path;
console.log(typeof orPicked);

// A falsy left short-circuits to itself, so the right is never reached and the result
// is the left operand, not a boolean.
const missing = process.env.BENTO_NO_SUCH_VAR && os;
console.log(typeof missing, missing === undefined);

// The same expression in the two other boolean positions, a ternary and a while.
console.log(os && path ? "both" : "neither");
let spins = 0;
while (os && path && spins < 2) spins++;
console.log(spins);
