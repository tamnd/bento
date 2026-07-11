const o: any = {};
Object.defineProperties(o, {
  a: { value: 1, enumerable: true },
  b: { value: 2, enumerable: false },
  c: { get: function () { return 3; }, enumerable: true },
});
console.log(o.a);
console.log(o.b);
console.log(o.c);
console.log(Object.keys(o).join(","));
console.log(Object.getOwnPropertyNames(o).join(","));
