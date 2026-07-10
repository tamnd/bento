class Calc {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
  async double(): Promise<number> {
    return this.x * 2;
  }
  static async unit(v: number): Promise<number> {
    return v + 1;
  }
}
console.log("start");
new Calc(21).double().then(v => console.log("double:" + String(v)));
Calc.unit(9).then(v => console.log("unit:" + String(v)));
console.log("end");
