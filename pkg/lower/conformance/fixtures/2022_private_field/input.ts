class Box {
  #count: number = 1;
  bump(): void {
    this.#count = this.#count + 4;
  }
  value(): number {
    return this.#count;
  }
}
const b = new Box();
b.bump();
b.bump();
console.log(String(b.value()));
