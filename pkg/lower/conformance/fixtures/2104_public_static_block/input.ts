class Registry {
  static total: number = 0;
  static {
    Registry.total = 10 + 5;
  }
}
console.log(Registry.total);
