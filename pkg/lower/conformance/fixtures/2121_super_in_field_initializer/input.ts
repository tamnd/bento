class Base {
  seed(): number {
    return 10;
  }
}
class Derived extends Base {
  value: number = super.seed() + 5;
}
console.log(String(new Derived().value));
