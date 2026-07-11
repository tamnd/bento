// An async arrow and an async function expression both return a promise the same way
// an async declaration does. A concise-bodied arrow returns its one expression, a
// block-bodied arrow returns through a return statement, and a function expression
// wraps its block, each settling the promise a .then callback reads after the
// synchronous run finishes.
const triple = async (n: number): Promise<number> => n * 3;
const inc = async function (n: number): Promise<number> {
  return n + 1;
};

console.log("start");
triple(4).then((v) => console.log(v));
inc(9).then((v) => console.log(v));
console.log("end");
