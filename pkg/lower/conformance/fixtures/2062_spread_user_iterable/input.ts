// A spread of a user iterable walks the iterator protocol, both in an array literal
// and in a call argument list. [99, ...counter, 88] drains the iterable between the
// fixed elements, and sum(100, ...counter, 200) collects the drained values into
// the rest array. The iterable is drained the same way the for...of loop pulls it,
// so the spliced values are the ones it yields, 0, 1, 2.
class Counter {
  i: number;
  stop: number;
  constructor(stop: number) {
    this.i = 0;
    this.stop = stop;
  }
  [Symbol.iterator](): Counter {
    return this;
  }
  next(): { value: number; done: boolean } {
    if (this.i >= this.stop) {
      return { value: 0, done: true };
    }
    const v = this.i;
    this.i = this.i + 1;
    return { value: v, done: false };
  }
}

const a = [99, ...new Counter(3), 88];
console.log(String(a.length));
for (const x of a) {
  console.log(String(x));
}

function sum(...xs: number[]): number {
  let s = 0;
  for (const x of xs) {
    s = s + x;
  }
  return s;
}
console.log(String(sum(100, ...new Counter(3), 200)));
