class Base {
  _v: number = 0;
  get value(): number {
    return this._v;
  }
  set value(n: number) {
    this._v = n;
  }
  describe(): string {
    return "base";
  }
}
class Derived extends Base {
  bump(): void {
    super.value = super.value + 5;
  }
  label(): string {
    return super.describe() + "-derived";
  }
}
const d = new Derived();
d.value = 10;
d.bump();
console.log(String(d.value));
console.log(d.label());
