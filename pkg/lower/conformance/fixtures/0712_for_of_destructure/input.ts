// A destructuring for...of over an array of arrays binds each name to a position of
// the element. Go has no destructuring, so it lowers to a range loop whose element is
// bound to a generated temporary and destructured at the top of the body. The range
// value is fresh each iteration, so the positional reads see that iteration's element
// with no reset. A name the body never reads is dropped rather than bound.
const pairs: number[][] = [[1, 2], [3, 4], [5, 6]];
for (const [a, b] of pairs) {
  console.log(a + " + " + b + " = " + (a + b));
}

const rows: number[][] = [[10, 100], [20, 200]];
for (const [a, b] of rows) {
  console.log(b);
}
