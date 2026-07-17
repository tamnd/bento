// A closure captures a typed local declared without an initializer. The checker does
// not police a read of such a local from inside a closure, so a plain Go var would read
// Go's zero where a closure that ran before the first assignment would observe undefined.
// It is safe all the same when the local is assigned by unconditional top-level code
// before any capturing closure is defined: the closure then sits after the assignment
// and cannot run before the slot holds a real value, and Go's by-reference capture reads
// that value. So the local lowers to a plain typed var rather than handing back.
function first(): number {
  let x: number;
  x = 3;
  const g = () => x;
  return g();
}
// The by-reference capture reads the latest value, including a reassignment made after
// the closure is defined.
function latest(): number {
  let x: number;
  x = 1;
  const g = () => x;
  x = 7;
  return g();
}
console.log(first()); // 3
console.log(latest()); // 7
