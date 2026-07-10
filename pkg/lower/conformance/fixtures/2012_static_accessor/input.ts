// A static get/set accessor pair backing a static field, read and written
// through the class name. The getter lowers to the package function CV and the
// setter to CSetV, and C.v routes a read to CV() and a write to CSetV(...).
class Counter {
  static _v: number = 10;
  static get v(): number {
    return Counter._v;
  }
  static set v(n: number) {
    Counter._v = n;
  }
}

console.log(String(Counter.v));
Counter.v = Counter.v + 5;
console.log(String(Counter.v));
Counter.v = 100;
console.log(String(Counter.v));
