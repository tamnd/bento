class Temperature {
  #celsius: number = 0;
  get #fahrenheit(): number {
    return this.#celsius * 9 / 5 + 32;
  }
  set #fahrenheit(f: number) {
    this.#celsius = (f - 32) * 5 / 9;
  }
  setF(f: number): void {
    this.#fahrenheit = f;
  }
  readF(): number {
    return this.#fahrenheit;
  }
  readC(): number {
    return this.#celsius;
  }
}
const t = new Temperature();
t.setF(212);
console.log(t.readC() + "," + t.readF());
