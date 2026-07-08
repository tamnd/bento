// An assignment is an expression whose value is the value assigned, so it can be
// read where a value is expected, not only run as a statement. A local target
// names its own slot, a class or object field stores through its selector, and a
// chained assignment settles every target from the value the innermost one holds.
// Each form below is read for its value inside console.log, so the printed result
// is the assignment expression's own value.

class Box {
  value = 0;
  store(v: number): number {
    return this.value = v;
  }
}
const b = new Box();
console.log(b.store(7), b.value);

const o = { n: 0, label: "" };
console.log((o.n = 5), o.n);
console.log((o.label = "hi"), o.label);

let x = 0;
let y = 0;
console.log(x = y = 9, x, y);

let s = "";
console.log(s = "set");
