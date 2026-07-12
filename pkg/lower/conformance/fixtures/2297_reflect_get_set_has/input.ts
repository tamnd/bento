// Reflect.get, Reflect.has, and Reflect.set are the reflective forms of a property
// read, an in probe, and a write. get and has climb the prototype chain, and set
// reports whether the write succeeded rather than throwing on a refused one.
const obj: any = { a: 1, b: 2 };
console.log(Reflect.get(obj, "a")); // 1
console.log(Reflect.get(obj, "missing")); // undefined, absent property reads undefined
console.log(Reflect.has(obj, "b")); // true
console.log(Reflect.has(obj, "missing")); // false

// An inherited property reads and probes through the chain.
const proto: any = { inherited: 42 };
const child: any = Object.create(proto);
console.log(Reflect.get(child, "inherited")); // 42
console.log(Reflect.has(child, "inherited")); // true

// set reports success and writes both an existing and a fresh own property.
console.log(Reflect.set(obj, "a", 10)); // true
console.log(Reflect.get(obj, "a")); // 10
console.log(Reflect.set(obj, "c", 3)); // true, a new own property
console.log(Reflect.get(obj, "c")); // 3

// A non-extensible target refuses a new property, and set returns false.
Object.preventExtensions(obj);
console.log(Reflect.set(obj, "d", 4)); // false
console.log(Reflect.has(obj, "d")); // false

// A non-writable data property refuses the write and keeps its value.
const locked: any = {};
Object.defineProperty(locked, "x", { value: 1, writable: false, configurable: true });
console.log(Reflect.set(locked, "x", 2)); // false
console.log(Reflect.get(locked, "x")); // 1
