// A module-level binding with no initializer is a name that exists from the top of
// the module and holds undefined until something assigns it. When a top-level
// function reads it, the binding has to reach package scope like any other, and a
// package var declared at its zero value says exactly that: there is no value to
// evaluate at package-init time and nothing to assign at its source position, so the
// statement contributes nothing to main and the assignments that follow do the work.
//
// This is the shape Node's test/common uses for a listener it installs once and
// removes later.

let pending: number;

function record(n: number): void {
  pending = n;
}

function readBack(): number {
  return pending;
}

record(7);
console.log(readBack());
record(12);
console.log(readBack());
