// A class that defines [Symbol.iterator] is a user iterable, and a for...of over
// an instance of it walks the iterator protocol: it calls [Symbol.iterator] once
// to obtain the iterator, then pulls a { value, done } result each turn and stops
// when done is true. This class is its own iterator, returning this from
// [Symbol.iterator] and answering next itself, the common self-iterating shape, so
// the loop prints each value it yields until it runs out.
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

for (const n of new Counter(3)) {
  console.log(String(n));
}
