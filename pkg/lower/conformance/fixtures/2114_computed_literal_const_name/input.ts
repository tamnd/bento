const mk = "run";
const fk = "count";
class C {
  [fk]: number = 5;
  [mk](): number {
    return this.count + 1;
  }
}
const c = new C();
console.log(String(c[mk]()));
console.log(String(c.run()));
