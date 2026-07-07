// A function expression used as a value is the function(){} form the test262
// assert prelude leans on: it binds the prelude's assert and compareArray to a
// const and passes function expressions on from there. A plain block-bodied,
// this-free expression is the same closure a block-body arrow lowers to, so it
// binds to a const, returns a value, and calls the same. The void-bodied one
// passed as a callback runs for its effect with no result.
const inc = function (x: number): number {
  return x + 1;
};
console.log(inc(2));

function run(cb: (a: number) => void): void {
  cb(41);
}

run(function (n: number): void {
  console.log(n + 1);
});
