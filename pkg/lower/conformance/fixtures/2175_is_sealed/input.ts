const a: any = { x: 1 };
console.log(Object.isSealed(a));
Object.preventExtensions(a);
console.log(Object.isSealed(a));

const b: any = { y: 1 };
Object.seal(b);
console.log(Object.isSealed(b));

const c: any = { z: 1 };
Object.freeze(c);
console.log(Object.isSealed(c));

const empty: any = {};
Object.preventExtensions(empty);
console.log(Object.isSealed(empty));
