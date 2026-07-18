// The multi-statement method box lowers a body of locals and branches, but a body that
// reads this needs the boxed receiver threaded in as the closure's this, which is a later
// slice. A multi-statement valueOf that reads this.base must hand the whole unit back
// rather than drop the receiver and compute a wrong number, so the decline is proven here.

const o: any = {
  valueOf() {
    let a = 1;
    return this.base + a;
  },
  base: 10,
};
console.log(o < 100);
