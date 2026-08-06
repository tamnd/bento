// An object in boolean position is always truthy: it carries no falsy member, so the
// checker settles the branch and the test collapses to the Go constant. That collapse
// drops the operand, which is right for a name or a field read and wrong for a call,
// whose effect has to fire whether or not anything reads what it returned.
//
// So a side-effecting always-truthy operand lowers to the constant behind the
// evaluation instead: the operand runs, its result is discarded, and the answer is
// still true. It is the same immediately invoked func the value-returning logical uses
// to hold a statement where an expression is wanted.

let calls = 0;

function build(): { a: number } {
  calls++;
  return { a: calls };
}

if (build()) console.log("truthy", calls);
console.log(!build(), calls);
console.log(build() ? "yes" : "no", calls);

// The same in the operand position of a logical, where the guard is the truthiness and
// the value is the operand: a truthy left short-circuits || to itself and lets && reach
// the right, so build runs once for the guard either way.
const flag = (build() && true) || false;
console.log(flag, calls);

// An array and a class instance are the same kind of always-truthy, and a function call
// returning one gets the same treatment.
function rows(): number[] {
  calls++;
  return [];
}

if (rows()) console.log("empty array is truthy", calls);

class Node2 {
  n = 1;
}

function node(): Node2 {
  calls++;
  return new Node2();
}

if (node()) console.log("instance is truthy", calls);

// The repeatable form is untouched: a plain binding still folds to the constant with no
// func around it, and the object is never evaluated twice.
const o = { a: 1 };
if (o) console.log("binding folds", calls);
