// reduce called with only a callback and no initial value seeds the accumulator
// from the first element and folds from the second. A single-element array returns
// that element without ever running the callback. The accumulator type matches the
// element type, unlike the initial-value form that can fold into a different type.
function sum(a: number[]): number {
  return a.reduce((acc, n) => acc + n);
}

function product(a: number[]): number {
  return a.reduce((acc, n) => acc * n);
}

function max(a: number[]): number {
  return a.reduce((acc, n) => (n > acc ? n : acc));
}

function run(): void {
  console.log(sum([1, 2, 3, 4, 5]));
  console.log(product([2, 3, 4]));
  console.log(max([3, 1, 4, 1, 5, 9, 2, 6]));
  console.log(sum([7]));
}

run();
