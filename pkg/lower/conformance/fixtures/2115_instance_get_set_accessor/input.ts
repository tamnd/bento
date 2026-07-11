class Temperature {
  _celsius: number = 0;
  get celsius(): number {
    return this._celsius;
  }
  set celsius(value: number) {
    this._celsius = value;
  }
  get fahrenheit(): number {
    return this._celsius * 1.8 + 32;
  }
}
const t = new Temperature();
t.celsius = 25;
console.log(String(t.celsius));
console.log(String(t.fahrenheit));
