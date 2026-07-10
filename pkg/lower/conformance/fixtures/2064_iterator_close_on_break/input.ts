// A for...of that breaks out of a user iterable closes the iterator: it calls the
// iterator's return() method on the early exit, the way the spec closes an iterable
// left mid-iteration. This iterable never reports done on its own, so the loop ends
// only by breaking, and return() prints a marker so the close is observable. The
// loop prints 0 and 1, breaks at 2 (which fires return(), printing "closed"), then
// control falls through to the line after the loop.
class Guarded {
  i: number = 0;
  [Symbol.iterator](): Guarded {
    return this;
  }
  next(): { value: number; done: boolean } {
    const v = this.i;
    this.i = this.i + 1;
    return { value: v, done: false };
  }
  return(): { value: number; done: boolean } {
    console.log("closed");
    return { value: 0, done: true };
  }
}

for (const x of new Guarded()) {
  if (x >= 2) {
    break;
  }
  console.log(String(x));
}
console.log("after");
