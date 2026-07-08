// A var is scoped to the whole function, not the block it is written in, so a var
// assigned inside a block and read after it is one binding. Bento emitted the
// in-block declaration as a Go short declaration, which left the name undefined at
// the read outside the block and unused inside it, so the emit did not build.
// Hoisting the var to the top of its scope declares it once and turns the in-block
// declaration into an assignment, the function-scoping JavaScript gives a var.
function label(n: number): string {
  if (n > 0) {
    var sign = "positive";
  } else {
    var sign = "non-positive";
  }
  return sign;
}
console.log(label(3));
console.log(label(-2));

// A var written in a catch block and read after the try is hoisted the same way at
// the module scope. The throw runs unconditionally, so the catch always assigns
// handled before the read, which is what lets the checker accept the later use.
let outcome = "";
try {
  throw "boom";
} catch (e) {
  var handled = 1;
} finally {
  outcome = "swept";
}
if (handled === 1) {
  console.log("caught");
}
console.log(outcome);
