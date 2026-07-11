const s = Symbol("hidden");
const o: any = { a: 1, b: 2 };
o[s] = 9;
Object.defineProperty(o, "c", { value: 3, enumerable: false });

console.log(Object.keys(o).join(","));
console.log(Object.getOwnPropertyNames(o).join(","));
console.log(Object.values(o).join(","));
console.log(Object.hasOwn(o, "c"));
console.log(Object.hasOwn(o, s));
