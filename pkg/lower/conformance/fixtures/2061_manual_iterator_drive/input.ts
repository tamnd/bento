// A test can drive an iterable by hand instead of through a for...of: it reads the
// iterator factory with obj[Symbol.iterator](), then pulls each { value, done }
// result with a direct next() call and stops when done is true. The manual walk
// lowers to the same two Go methods, SymbolIterator and Next, the loop calls, so
// the values it reads out are the ones the iterable yields, 0 then 1.
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

const it = new Counter(2)[Symbol.iterator]();
let r = it.next();
console.log(String(r.value));
r = it.next();
console.log(String(r.value));
console.log(String(it.next().done));
