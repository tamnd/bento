class Counter {
  step(): number {
    return this.#delta();
  }
  #delta(): number {
    return 2;
  }
  delta(): number {
    return 100;
  }
}
const c = new Counter();
console.log(String(c.step()));
console.log(String(c.delta()));
