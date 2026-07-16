// `"key" in obj` on a static fixed-shape receiver has no runtime object to probe, but a
// required own property is always present, so the membership test is a compile-time
// true. Only that branch folds: an optional member, a member the shape does not
// declare (which JavaScript may still find on Object.prototype), a non-literal key, and
// a side-effecting receiver all keep the honest handback, so this fixture drives only
// the provable-present cases. The receiver is still evaluated where it names a binding
// with no side effect, so nothing observable is dropped.

interface Point {
  x: number;
  y: number;
}

function hasX(p: Point): boolean {
  return "x" in p;
}

class Counter {
  value: number = 0;
  step(): number {
    this.value = this.value + 1;
    return this.value;
  }
}

const obj = { a: 1, b: 2 };
const p: Point = { x: 3, y: 4 };
const c = new Counter();

// A required data member folds to true on an object literal binding.
console.log("a" in obj);
// A required member folds through an interface-typed binding and inside a function.
console.log(hasX(p));
// A required data member folds on a class instance.
console.log("value" in c);
// A required method folds on a class instance, the property being on its prototype.
console.log("step" in c);
