// Reflect.deleteProperty is the reflective delete, reporting whether the removal
// succeeded. A configurable property removes and reports true, while a
// non-configurable one survives and reports false.
const obj: any = { a: 1, b: 2, c: 3 };
console.log(Reflect.deleteProperty(obj, "b")); // true
console.log(Reflect.has(obj, "a")); // true
console.log(Reflect.has(obj, "b")); // false

// A non-configurable property survives the delete and reports false.
const locked: any = {};
Object.defineProperty(locked, "x", { value: 1, configurable: false });
console.log(Reflect.deleteProperty(locked, "x")); // false
console.log(Reflect.has(locked, "x")); // true

// Reflect.ownKeys collects every own key, string and symbol alike. Its result is a
// (string | symbol)[], a union-element array bento does not yet consume, so the
// call lowers on its own while the enumeration order is pinned by the runtime unit
// test TestReflectOwnKeys.
Reflect.ownKeys(obj);
