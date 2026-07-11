const o: any = {};
Object.defineProperty(o, "b", { value: 1, writable: true, enumerable: true, configurable: true });
Object.defineProperty(o, "a", { value: 2, writable: false, enumerable: false, configurable: true });
o["2"] = 30;
o["1"] = 10;
console.log(Object.getOwnPropertyNames(o).join(","));

const all: any = Object.getOwnPropertyDescriptors(o);
console.log(Object.getOwnPropertyNames(all).join(","));
console.log(all.b.value);
console.log(all.b.writable);
console.log(all.a.value);
console.log(all.a.enumerable);
