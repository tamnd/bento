class Box {
  _v: number = 0;
  get ["val"](): number {
    return this._v;
  }
  set ["val"](n: number) {
    this._v = n;
  }
}
const b = new Box();
b["val"] = 9;
console.log(String(b["val"]));
