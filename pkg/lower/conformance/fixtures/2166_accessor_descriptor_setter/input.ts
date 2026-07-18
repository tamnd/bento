const o: any = {};
let backing: number = 10;
Object.defineProperty(o, "x", {
  get: function () {
    return backing;
  },
  set: function (v: number) {
    backing = v * 2;
  },
  enumerable: true,
  configurable: true,
});
console.log(String(o.x));
o.x = 21;
console.log(String(o.x));
const d: any = Object.getOwnPropertyDescriptor(o, "x");
console.log(String(typeof d.get), String(typeof d.set));
