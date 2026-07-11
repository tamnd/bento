const o: any = { a: 1, b: 2 };
console.log(Object.keys(o).join(","));
console.log(Object.getOwnPropertyNames(o).join(","));
console.log(Object.values(o).join(","));
console.log(Object.hasOwn(o, "a"));
console.log(Object.hasOwn(o, "z"));
