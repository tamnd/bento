// Array destructuring off a user iterable walks the iterator protocol: the source
// is drained into an array once, then each target binds a value by index the same
// way it binds off a real array. So const [a, b, c] = counter binds the first three
// values the iterable yields, 0, 1, 2.
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

const [a, b, c] = new Counter(5);
console.log(String(a));
console.log(String(b));
console.log(String(c));
