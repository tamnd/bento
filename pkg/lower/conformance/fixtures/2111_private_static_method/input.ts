class Greeter {
  static #greeting(): string {
    return "hi";
  }
  static greet(): string {
    return Greeter.#greeting() + " there";
  }
}
console.log(Greeter.greet());
