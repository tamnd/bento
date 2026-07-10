// A for...of over arguments ranges the store the body materialized from its
// parameters, yielding each argument in order. The count form drops the unused loop
// binding and ranges only to drive the iteration; the element form binds each
// argument and prints it. This proves iterating the arguments object end to end.
function count(a: number, b: number, c: number): number {
  let n = 0;
  for (const x of arguments) {
    n++;
  }
  return n;
}

// arguments has three elements, so the count is 3.
console.log(count(1, 2, 3));

function each(a: number, b: number): void {
  for (const x of arguments) {
    console.log(x);
  }
}

// each argument is printed in order: 7 then 8.
each(7, 8);
