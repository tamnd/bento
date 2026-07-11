class Counter {
  static #count: number = 0;
  static bump(): number {
    Counter.#count = Counter.#count + 1;
    return Counter.#count;
  }
}
Counter.bump();
Counter.bump();
console.log(Counter.bump());
