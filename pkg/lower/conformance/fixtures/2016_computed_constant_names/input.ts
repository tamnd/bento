class C {
  v: number = 0;
  constructor() { this.v = 1; }
  ["constructor"](): number { return 99; }
  ["plain"](): number { return 7; }
}
const c = new C();
console.log(String(c.v));
console.log(String(c["constructor"]()));
console.log(String(c["plain"]()));
