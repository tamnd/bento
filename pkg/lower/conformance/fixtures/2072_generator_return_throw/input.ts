// A generator driven by hand through return() and throw(), the two early closes a
// for...of cannot express. return(v) closes a suspended generator, unwinding it through
// its finally block and completing with v, so the { value, done } it packs reads
// { 99, true } after the finally logs its cleanup. throw(e) raises e at the suspended
// yield, so a try/catch in the body catches it and the generator resumes past the catch,
// yielding again; the result the throw packs carries that next yielded value.
function* guarded(): Generator<number> {
  try {
    yield 1;
    yield 2;
  } finally {
    console.log("cleanup");
  }
}

const g = guarded();
console.log("value " + String(g.next().value));
const done = g.return(99);
console.log("return " + String(done.value) + " " + String(done.done));

function* recover(): Generator<number> {
  try {
    yield 1;
  } catch (err) {
    console.log("caught");
    yield 2;
  }
}

const r = recover();
console.log("first " + String(r.next().value));
console.log("after " + String(r.throw(new Error("boom")).value));
