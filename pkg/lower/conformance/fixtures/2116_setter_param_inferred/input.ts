class Box {
  _v: number = 0;
  get value(): number {
    return this._v;
  }
  set value(v) {
    this._v = v + 1;
  }
}
const b = new Box();
b.value = 10;
console.log(String(b.value));
