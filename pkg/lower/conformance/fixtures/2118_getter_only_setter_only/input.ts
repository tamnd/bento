class ReadOnly {
  get answer(): number {
    return 42;
  }
}
class WriteOnly {
  _log: string = "";
  set entry(v: string) {
    this._log = this._log + v;
  }
  dump(): string {
    return this._log;
  }
}
const r = new ReadOnly();
console.log(String(r.answer));
const w = new WriteOnly();
w.entry = "a";
w.entry = "b";
console.log(w.dump());
