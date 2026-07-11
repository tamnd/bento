class Counter {
  static instances: number = 0;
  static bump(): void {
    Counter.instances = Counter.instances + 1;
  }
  static {
    Counter.bump();
    Counter.bump();
  }
}
console.log(Counter.instances);
