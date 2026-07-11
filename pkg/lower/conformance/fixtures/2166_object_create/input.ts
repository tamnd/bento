const proto: any = {};
const o: any = Object.create(proto, {
  a: { value: 1, enumerable: true, writable: true, configurable: true },
  b: { value: 2, enumerable: false, writable: true, configurable: true },
});
console.log(o.a);
console.log(o.b);
console.log(Object.getOwnPropertyNames(o).join(","));
console.log(Object.keys(o).join(","));
