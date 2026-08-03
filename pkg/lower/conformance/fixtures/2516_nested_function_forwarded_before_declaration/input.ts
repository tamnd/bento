// A nested function declaration whose name is read before its own position binds
// ahead of the reader, which fixture 2514 covers. This is what that move runs into
// and how it gets past.
//
// A sibling function declared below the destination does not block it: that sibling
// takes a binding at the top of the block too, so it exists wherever the move lands.
// All it owes is that the moving body reads it later rather than while it runs,
// which `one` does here by reading `two` from inside a callback.
//
// A local declared below the destination does block it, and the answer is a
// forwarder. `step` is handed to a call as a value before it is declared and its
// body reads a `bump` declared after that call, so `step` binds a closure that does
// nothing but call `stepImpl`, and `stepImpl` takes the real body at the
// declaration's own position where `bump` exists. The value passed out early is the
// one that runs the body later, which is what the source promised.
//
// A call, rather than a value read, before such a declaration is what still hands
// back; that is fixture 2464.

function later(f: () => void): void {
  f();
}

function chain(n: number): number {
  const seed = one(n);
  function one(x: number): number {
    let held = 0;
    later(() => {
      held = two(x);
    });
    return held + 1;
  }
  function two(x: number): number {
    return x * 2;
  }
  return seed;
}

let saved: (x: number) => number = (x: number) => x;

function keep(f: (x: number) => number): void {
  saved = f;
}

function forwarded(n: number): number {
  // step is passed along as a value here, above its declaration.
  keep(step);
  // And it reads this, which the source declares after the call, so its binding
  // cannot move up here and the name holds a forwarder instead.
  const bump = 3;
  function step(x: number): number {
    return x + bump;
  }
  return n;
}

console.log(chain(4));
forwarded(1);
console.log(saved(4));
